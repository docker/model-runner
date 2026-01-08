package responses

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// StreamingResponseWriter transforms chat completion SSE events to Responses API SSE events.
type StreamingResponseWriter struct {
	w              http.ResponseWriter
	flusher        http.Flusher
	response       *Response
	store          *Store
	sequenceNumber int
	headersSent    bool
	buffer         strings.Builder

	// State for building the response
	currentItemID      string
	currentContentIdx  int
	accumulatedContent strings.Builder
	toolCalls          []OutputItem
}

// NewStreamingResponseWriter creates a new streaming response writer.
func NewStreamingResponseWriter(w http.ResponseWriter, resp *Response, store *Store) *StreamingResponseWriter {
	flusher, _ := w.(http.Flusher)
	return &StreamingResponseWriter{
		w:        w,
		flusher:  flusher,
		response: resp,
		store:    store,
	}
}

// Header returns the header map.
func (s *StreamingResponseWriter) Header() http.Header {
	return s.w.Header()
}

// WriteHeader writes the HTTP status code.
func (s *StreamingResponseWriter) WriteHeader(statusCode int) {
	if s.headersSent {
		return
	}
	s.headersSent = true

	if statusCode != http.StatusOK {
		// Send error event before writing the status code
		s.response.Status = StatusFailed
		s.sendEvent(EventError, &StreamEvent{
			Type:           EventError,
			SequenceNumber: s.nextSeq(),
			Error: &ErrorDetail{
				Code:    "upstream_error",
				Message: fmt.Sprintf("Upstream service returned status code: %d", statusCode),
			},
		})

		// Send response.failed event
		s.sendEvent(EventResponseFailed, &StreamEvent{
			Type:           EventResponseFailed,
			SequenceNumber: s.nextSeq(),
			Response:       s.response,
		})

		// Store the failed response
		if s.store != nil {
			s.store.Save(s.response)
		}

		s.w.WriteHeader(statusCode)
		return
	}

	s.w.Header().Set("Content-Type", "text/event-stream")
	s.w.Header().Set("Cache-Control", "no-cache")
	s.w.Header().Set("Connection", "keep-alive")
	s.w.WriteHeader(statusCode)

	// Send response.created event
	s.sendEvent(EventResponseCreated, &StreamEvent{
		Type:           EventResponseCreated,
		SequenceNumber: s.nextSeq(),
		Response:       s.response,
	})

	// Send response.in_progress event
	s.response.Status = StatusInProgress
	s.sendEvent(EventResponseInProgress, &StreamEvent{
		Type:           EventResponseInProgress,
		SequenceNumber: s.nextSeq(),
		Response:       s.response,
	})
}

// Write processes incoming chat completion SSE data.
func (s *StreamingResponseWriter) Write(data []byte) (int, error) {
	if !s.headersSent {
		s.WriteHeader(http.StatusOK)
	}

	// Buffer the data
	s.buffer.Write(data)

	// Process complete lines
	bufferStr := s.buffer.String()
	lines := strings.Split(bufferStr, "\n")

	// Keep incomplete line in buffer
	if !strings.HasSuffix(bufferStr, "\n") && len(lines) > 0 {
		s.buffer.Reset()
		s.buffer.WriteString(lines[len(lines)-1])
		lines = lines[:len(lines)-1]
	} else {
		s.buffer.Reset()
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		dataStr := strings.TrimPrefix(line, "data: ")

		// Detect and propagate upstream metadata-only final chunks (e.g. usage, finish_reason)
		// before we consider the stream finalized. This ensures that Response.Usage and
		// Response.Status.FinishReason are preserved even when the final chunk has no text.
		if dataStr != "" && dataStr != "[DONE]" {
			var metaEnvelope struct {
				Usage   *Usage `json:"usage,omitempty"`
				Choices []struct {
					FinishReason string          `json:"finish_reason,omitempty"`
					Delta        json.RawMessage `json:"delta,omitempty"`
				} `json:"choices,omitempty"`
			}

			if err := json.Unmarshal([]byte(dataStr), &metaEnvelope); err != nil {
				// Send error event for malformed JSON in metadata chunk
				s.response.Status = StatusFailed
				s.sendEvent(EventError, &StreamEvent{
					Type:           EventError,
					SequenceNumber: s.nextSeq(),
					Error: &ErrorDetail{
						Code:    "parse_error",
						Message: fmt.Sprintf("Failed to parse SSE metadata chunk: %v", err),
					},
				})

				// Send response.failed event
				s.sendEvent(EventResponseFailed, &StreamEvent{
					Type:           EventResponseFailed,
					SequenceNumber: s.nextSeq(),
					Response:       s.response,
				})

				// Store the failed response
				if s.store != nil {
					s.store.Save(s.response)
				}
				return len(data), nil
			}

			// If upstream sent usage only in the final chunk, capture it once.
			if metaEnvelope.Usage != nil && s.response != nil && s.response.Usage == nil {
				s.response.Usage = &Usage{
					InputTokens:  metaEnvelope.Usage.InputTokens,
					OutputTokens: metaEnvelope.Usage.OutputTokens,
					TotalTokens:  metaEnvelope.Usage.TotalTokens,
				}
			}

			// If we have a finish_reason but an empty delta, treat this as a
			// metadata-only final chunk and propagate the finish state.
			if s.response != nil {
				for _, choice := range metaEnvelope.Choices {
					if choice.FinishReason != "" {
						s.response.FinishReason = choice.FinishReason
						// Update status based on finish reason
						switch choice.FinishReason {
						case "stop", "tool_calls":
							s.response.Status = StatusCompleted
						case "length":
							s.response.Status = StatusCompleted // or potentially a different status for truncation
							if s.response.IncompleteDetails == nil {
								s.response.IncompleteDetails = &IncompleteDetails{Reason: "max_tokens"}
							}
						case "content_filter":
							s.response.Status = StatusFailed
							if s.response.Error == nil {
								s.response.Error = &ErrorDetail{
									Code:    "content_filter",
									Message: "Content filtered",
								}
							}
						}
						break
					}
				}
			}
		}

		if dataStr == "[DONE]" {
			s.finalize()
			continue
		}

		s.processChunk(dataStr)
	}

	return len(data), nil
}

// processChunk processes a single SSE chunk from chat completions.
func (s *StreamingResponseWriter) processChunk(dataStr string) {
	var chunk ChatStreamChunk
	if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
		// Send error event for malformed JSON
		s.response.Status = StatusFailed
		s.sendEvent(EventError, &StreamEvent{
			Type:           EventError,
			SequenceNumber: s.nextSeq(),
			Error: &ErrorDetail{
				Code:    "parse_error",
				Message: fmt.Sprintf("Failed to parse SSE chunk: %v", err),
			},
		})

		// Send response.failed event
		s.sendEvent(EventResponseFailed, &StreamEvent{
			Type:           EventResponseFailed,
			SequenceNumber: s.nextSeq(),
			Response:       s.response,
		})

		// Store the failed response
		if s.store != nil {
			s.store.Save(s.response)
		}
		return
	}

	if len(chunk.Choices) == 0 {
		return
	}

	delta := chunk.Choices[0].Delta

	// Handle tool calls
	if len(delta.ToolCalls) > 0 {
		s.handleToolCallDelta(delta.ToolCalls)
		return
	}

	// Handle content delta
	if delta.Content != "" {
		s.handleContentDelta(delta.Content)
	}
}

// handleContentDelta handles a content delta from the chat completion stream.
func (s *StreamingResponseWriter) handleContentDelta(content string) {
	// Initialize message item if needed
	if s.currentItemID == "" {
		s.currentItemID = GenerateMessageID()
		s.currentContentIdx = 0

		// Send output_item.added
		item := &OutputItem{
			ID:   s.currentItemID,
			Type: ItemTypeMessage,
			Role: "assistant",
			Content: []ContentPart{{
				Type:        ContentTypeOutputText,
				Text:        "",
				Annotations: []Annotation{},
			}},
			Status: StatusInProgress,
		}
		s.sendEvent(EventOutputItemAdded, &StreamEvent{
			Type:           EventOutputItemAdded,
			SequenceNumber: s.nextSeq(),
			Item:           item,
			OutputIndex:    0,
		})

		// Send content_part.added
		s.sendEvent(EventContentPartAdded, &StreamEvent{
			Type:           EventContentPartAdded,
			SequenceNumber: s.nextSeq(),
			ItemID:         s.currentItemID,
			OutputIndex:    0,
			ContentIndex:   0,
			Part: &ContentPart{
				Type:        ContentTypeOutputText,
				Text:        "",
				Annotations: []Annotation{},
			},
		})
	}

	// Accumulate content
	s.accumulatedContent.WriteString(content)

	// Send output_text.delta
	s.sendEvent(EventOutputTextDelta, &StreamEvent{
		Type:           EventOutputTextDelta,
		SequenceNumber: s.nextSeq(),
		ItemID:         s.currentItemID,
		OutputIndex:    0,
		ContentIndex:   0,
		Delta:          content,
	})
}

// handleToolCallDelta handles tool call deltas from the chat completion stream.
func (s *StreamingResponseWriter) handleToolCallDelta(toolCalls []ChatToolCall) {
	for _, tc := range toolCalls {
		// Find or create the tool call item
		var item *OutputItem
		for i := range s.toolCalls {
			if s.toolCalls[i].CallID == tc.ID {
				item = &s.toolCalls[i]
				break
			}
		}

		if item == nil {
			// New tool call
			callID := tc.ID
			if callID == "" {
				callID = GenerateCallID()
			}
			newItem := OutputItem{
				ID:        GenerateItemID(),
				Type:      ItemTypeFunctionCall,
				CallID:    callID,
				Name:      tc.Function.Name,
				Arguments: "",
				Status:    StatusInProgress,
			}
			s.toolCalls = append(s.toolCalls, newItem)
			item = &s.toolCalls[len(s.toolCalls)-1]

			// Send output_item.added for function call
			s.sendEvent(EventOutputItemAdded, &StreamEvent{
				Type:           EventOutputItemAdded,
				SequenceNumber: s.nextSeq(),
				Item:           item,
				OutputIndex:    len(s.toolCalls) - 1,
			})
		}

		// Accumulate arguments
		if tc.Function.Arguments != "" {
			item.Arguments += tc.Function.Arguments

			// Send function_call_arguments.delta
			s.sendEvent(EventFunctionCallArgsDelta, &StreamEvent{
				Type:           EventFunctionCallArgsDelta,
				SequenceNumber: s.nextSeq(),
				ItemID:         item.ID,
				OutputIndex:    len(s.toolCalls) - 1,
				Delta:          tc.Function.Arguments,
			})
		}
	}
}

// finalize completes the streaming response.
func (s *StreamingResponseWriter) finalize() {
	// Finalize any accumulated content
	if s.currentItemID != "" {
		finalText := s.accumulatedContent.String()

		// Send output_text.done
		s.sendEvent(EventOutputTextDone, &StreamEvent{
			Type:           EventOutputTextDone,
			SequenceNumber: s.nextSeq(),
			ItemID:         s.currentItemID,
			OutputIndex:    0,
			ContentIndex:   0,
			Part: &ContentPart{
				Type:        ContentTypeOutputText,
				Text:        finalText,
				Annotations: []Annotation{},
			},
		})

		// Send content_part.done
		s.sendEvent(EventContentPartDone, &StreamEvent{
			Type:           EventContentPartDone,
			SequenceNumber: s.nextSeq(),
			ItemID:         s.currentItemID,
			OutputIndex:    0,
			ContentIndex:   0,
			Part: &ContentPart{
				Type:        ContentTypeOutputText,
				Text:        finalText,
				Annotations: []Annotation{},
			},
		})

		// Send output_item.done for message
		s.sendEvent(EventOutputItemDone, &StreamEvent{
			Type:           EventOutputItemDone,
			SequenceNumber: s.nextSeq(),
			OutputIndex:    0,
			Item: &OutputItem{
				ID:   s.currentItemID,
				Type: ItemTypeMessage,
				Role: "assistant",
				Content: []ContentPart{{
					Type:        ContentTypeOutputText,
					Text:        finalText,
					Annotations: []Annotation{},
				}},
				Status: StatusCompleted,
			},
		})

		// Add to response output
		s.response.Output = append(s.response.Output, OutputItem{
			ID:   s.currentItemID,
			Type: ItemTypeMessage,
			Role: "assistant",
			Content: []ContentPart{{
				Type:        ContentTypeOutputText,
				Text:        finalText,
				Annotations: []Annotation{},
			}},
			Status: StatusCompleted,
		})
		s.response.OutputText = finalText
	}

	// Finalize tool calls
	for i, tc := range s.toolCalls {
		// Send function_call_arguments.done
		s.sendEvent(EventFunctionCallArgsDone, &StreamEvent{
			Type:           EventFunctionCallArgsDone,
			SequenceNumber: s.nextSeq(),
			ItemID:         tc.ID,
			OutputIndex:    i,
			Delta:          tc.Arguments,
		})

		// Send output_item.done for function call
		tc.Status = StatusCompleted
		s.sendEvent(EventOutputItemDone, &StreamEvent{
			Type:           EventOutputItemDone,
			SequenceNumber: s.nextSeq(),
			OutputIndex:    i,
			Item:           &tc,
		})

		// Add to response output
		s.response.Output = append(s.response.Output, tc)
	}

	// Update response status
	s.response.Status = StatusCompleted

	// Send response.completed
	s.sendEvent(EventResponseCompleted, &StreamEvent{
		Type:           EventResponseCompleted,
		SequenceNumber: s.nextSeq(),
		Response:       s.response,
	})

	// Store the final response
	if s.store != nil {
		s.store.Save(s.response)
	}
}

// sendEvent sends an SSE event.
func (s *StreamingResponseWriter) sendEvent(eventType string, event *StreamEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	_, _ = fmt.Fprintf(s.w, "event: %s\n", eventType)
	_, _ = fmt.Fprintf(s.w, "data: %s\n\n", data)

	if s.flusher != nil {
		s.flusher.Flush()
	}
}

// nextSeq returns the next sequence number.
func (s *StreamingResponseWriter) nextSeq() int {
	s.sequenceNumber++
	return s.sequenceNumber
}

// NonStreamingResponseCapture captures a non-streaming response.
type NonStreamingResponseCapture struct {
	StatusCode int
	Headers    http.Header
	Body       strings.Builder
}

// NewNonStreamingResponseCapture creates a new response capture.
func NewNonStreamingResponseCapture() *NonStreamingResponseCapture {
	return &NonStreamingResponseCapture{
		StatusCode: http.StatusOK,
		Headers:    make(http.Header),
	}
}

// Header returns the header map.
func (c *NonStreamingResponseCapture) Header() http.Header {
	return c.Headers
}

// Write writes data to the body.
func (c *NonStreamingResponseCapture) Write(data []byte) (int, error) {
	return c.Body.Write(data)
}

// WriteHeader sets the status code.
func (c *NonStreamingResponseCapture) WriteHeader(statusCode int) {
	c.StatusCode = statusCode
}

// ProcessSSEStream reads an SSE stream from a reader and processes it.
func ProcessSSEStream(reader *bufio.Reader, handler func(event, data string)) error {
	var currentEvent string
	var currentData strings.Builder

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			// Process any remaining data
			if currentData.Len() > 0 {
				handler(currentEvent, currentData.String())
			}
			return err
		}

		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			// Empty line signals end of event
			if currentData.Len() > 0 {
				handler(currentEvent, currentData.String())
				currentEvent = ""
				currentData.Reset()
			}
			continue
		}

		if strings.HasPrefix(line, "event:") {
			currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			data = strings.TrimPrefix(data, " ")
			currentData.WriteString(data)
		}
	}
}

// CreateErrorResponse creates an error response.
func CreateErrorResponse(respID, model, code, message string) *Response {
	resp := NewResponse(respID, model)
	resp.Status = StatusFailed
	resp.CreatedAt = float64(time.Now().Unix())
	resp.Error = &ErrorDetail{
		Code:    code,
		Message: message,
	}
	return resp
}
