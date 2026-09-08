package responses

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var responseTextFormatNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ChatCompletionRequest represents an OpenAI chat completion request.
type ChatCompletionRequest struct {
	Model             string              `json:"model"`
	Messages          []ChatMessage       `json:"messages"`
	Tools             []ChatTool          `json:"tools,omitempty"`
	ToolChoice        interface{}         `json:"tool_choice,omitempty"`
	Temperature       *float64            `json:"temperature,omitempty"`
	TopP              *float64            `json:"top_p,omitempty"`
	MaxTokens         *int                `json:"max_tokens,omitempty"`
	Stream            bool                `json:"stream,omitempty"`
	User              string              `json:"user,omitempty"`
	ParallelToolCalls *bool               `json:"parallel_tool_calls,omitempty"`
	ResponseFormat    *ChatResponseFormat `json:"response_format,omitempty"`
}

// ChatResponseFormat configures output formatting in chat completions.
type ChatResponseFormat struct {
	Type       string          `json:"type"`
	JSONSchema *ChatJSONSchema `json:"json_schema,omitempty"`
}

// ChatJSONSchema is the chat completions JSON schema response_format payload.
type ChatJSONSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema"`
	Strict      *bool           `json:"strict,omitempty"`
}

// ChatMessage represents a message in the chat completion format.
type ChatMessage struct {
	Role       string         `json:"role"`
	Content    interface{}    `json:"content"` // string or []ContentPart
	Name       string         `json:"name,omitempty"`
	ToolCalls  []ChatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

// ChatContentPart represents a content part in chat format.
type ChatContentPart struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	ImageURL *ChatImageURL `json:"image_url,omitempty"`
}

// ChatImageURL represents an image URL in chat format.
type ChatImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// ChatTool represents a tool in chat completion format.
type ChatTool struct {
	Type     string       `json:"type"`
	Function ChatFunction `json:"function"`
}

// ChatFunction represents a function definition.
type ChatFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

// ChatToolCall represents a tool call in chat format.
type ChatToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ChatFunctionCall `json:"function"`
}

// ChatFunctionCall represents a function call.
type ChatFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatCompletionResponse represents an OpenAI chat completion response.
type ChatCompletionResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
	Usage   *ChatUsage   `json:"usage,omitempty"`
}

// ChatChoice represents a choice in the chat completion response.
type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// ChatUsage represents token usage in chat completion format.
type ChatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatStreamChunk represents a streaming chunk from chat completions.
type ChatStreamChunk struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []ChatStreamChoice `json:"choices"`
	Usage   *ChatUsage         `json:"usage,omitempty"`
}

// ChatStreamChoice represents a choice in a streaming chunk.
type ChatStreamChoice struct {
	Index        int       `json:"index"`
	Delta        ChatDelta `json:"delta"`
	FinishReason *string   `json:"finish_reason"`
}

// ChatDelta represents the delta in a streaming chunk.
type ChatDelta struct {
	Role             string         `json:"role,omitempty"`
	Content          string         `json:"content,omitempty"`
	ToolCalls        []ChatToolCall `json:"tool_calls,omitempty"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
}

// TransformRequestToChatCompletion converts a Responses API request to a chat completion request.
func TransformRequestToChatCompletion(req *CreateRequest, store *Store) (*ChatCompletionRequest, error) {
	chatReq := &ChatCompletionRequest{
		Model:             req.Model,
		Temperature:       req.Temperature,
		TopP:              req.TopP,
		MaxTokens:         req.MaxOutputTokens,
		Stream:            req.Stream,
		User:              req.User,
		ParallelToolCalls: req.ParallelToolCalls,
		ToolChoice:        req.ToolChoice,
	}

	responseFormat, err := responseFormatFromText(req.Text)
	if err != nil {
		return nil, err
	}
	chatReq.ResponseFormat = responseFormat

	// Convert tools
	if len(req.Tools) > 0 {
		chatReq.Tools = make([]ChatTool, 0, len(req.Tools))
		for _, tool := range req.Tools {
			if tool.Type == "function" {
				chatTool := ChatTool{
					Type: "function",
				}
				if tool.Function != nil {
					chatTool.Function = ChatFunction{
						Name:        tool.Function.Name,
						Description: tool.Function.Description,
						Parameters:  tool.Function.Parameters,
					}
				} else {
					chatTool.Function = ChatFunction{
						Name:        tool.Name,
						Description: tool.Description,
						Parameters:  tool.Parameters,
					}
				}
				chatReq.Tools = append(chatReq.Tools, chatTool)
				continue
			}

			return nil, fmt.Errorf("tool type %q is not supported by Docker Model Runner Responses compatibility layer", tool.Type)
		}
	}

	// Build messages array
	var messages []ChatMessage

	// Add system message from instructions
	if req.Instructions != "" {
		messages = append(messages, ChatMessage{
			Role:    "system",
			Content: req.Instructions,
		})
	}

	// If there's a previous response, include its conversation
	if req.PreviousResponseID != "" {
		if store == nil {
			return nil, fmt.Errorf("previous_response_id %q cannot be used because response storage is unavailable", req.PreviousResponseID)
		}
		prevResp, ok := store.Get(req.PreviousResponseID)
		if !ok {
			return nil, fmt.Errorf("previous_response_id %q was not found", req.PreviousResponseID)
		}

		// Recursively get the conversation history
		prevMessages := getConversationHistory(prevResp, store)
		messages = append(messages, prevMessages...)
	}

	// Parse and convert input
	inputMessages, err := parseInput(req.Input)
	if err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}
	messages = append(messages, inputMessages...)

	chatReq.Messages = messages
	return chatReq, nil
}

func responseFormatFromText(text *ResponseTextConfig) (*ChatResponseFormat, error) {
	if text == nil || text.Format == nil {
		return nil, nil
	}

	format := text.Format
	switch format.Type {
	case "", "text":
		return nil, nil
	case "json_object":
		return &ChatResponseFormat{Type: "json_object"}, nil
	case "json_schema":
		if format.Name == "" {
			return nil, fmt.Errorf("text.format.name is required when text.format.type is json_schema")
		}
		if !responseTextFormatNamePattern.MatchString(format.Name) {
			return nil, fmt.Errorf("text.format.name must contain only letters, numbers, underscores, and dashes, with a maximum length of 64")
		}
		if strings.TrimSpace(string(format.Schema)) == "" {
			return nil, fmt.Errorf("text.format.schema is required when text.format.type is json_schema")
		}

		var schemaValue any
		if err := json.Unmarshal(format.Schema, &schemaValue); err != nil {
			return nil, fmt.Errorf("text.format.schema must be a valid JSON object: %w", err)
		}
		if _, ok := schemaValue.(map[string]any); !ok {
			return nil, fmt.Errorf("text.format.schema must be a JSON object")
		}

		return &ChatResponseFormat{
			Type: "json_schema",
			JSONSchema: &ChatJSONSchema{
				Name:        format.Name,
				Description: format.Description,
				Schema:      format.Schema,
				Strict:      format.Strict,
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported text.format.type %q", format.Type)
	}
}

// parseInput parses the input field which can be a string or array of items.
func parseInput(input json.RawMessage) ([]ChatMessage, error) {
	if len(input) == 0 {
		return nil, nil
	}

	// Try parsing as a string first
	var strInput string
	if err := json.Unmarshal(input, &strInput); err == nil {
		return []ChatMessage{{
			Role:    "user",
			Content: strInput,
		}}, nil
	}

	// Try parsing as an array of input items
	var items []InputItem
	if err := json.Unmarshal(input, &items); err != nil {
		return nil, fmt.Errorf("input must be a string or array of items: %w", err)
	}

	return convertInputItems(items)
}

// convertInputItems converts input items to chat messages.
func convertInputItems(items []InputItem) ([]ChatMessage, error) {
	var messages []ChatMessage

	for _, item := range items {
		switch {
		case item.Type == ItemTypeFunctionCallOutput || item.CallID != "":
			// Function call output -> tool message
			messages = append(messages, ChatMessage{
				Role:       "tool",
				Content:    item.Output,
				ToolCallID: item.CallID,
			})

		case item.Role != "":
			// Message-style input
			msg := ChatMessage{
				Role: item.Role,
			}

			// Parse content
			if len(item.Content) > 0 {
				content, err := parseContent(item.Content)
				if err != nil {
					return nil, err
				}
				msg.Content = content
			}

			messages = append(messages, msg)

		default:
			// Try to interpret as a simple message
			if len(item.Content) > 0 {
				content, err := parseContent(item.Content)
				if err != nil {
					return nil, err
				}
				messages = append(messages, ChatMessage{
					Role:    "user",
					Content: content,
				})
			}
		}
	}

	return messages, nil
}

// parseContent parses content which can be a string or array of content parts.
func parseContent(content json.RawMessage) (interface{}, error) {
	// Try string first
	var strContent string
	if err := json.Unmarshal(content, &strContent); err == nil {
		return strContent, nil
	}

	// Try array of content parts
	var parts []InputContentPart
	if err := json.Unmarshal(content, &parts); err != nil {
		return nil, fmt.Errorf("content must be string or array: %w", err)
	}

	// Convert to chat format content parts
	chatParts := make([]ChatContentPart, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case ContentTypeInputText, "text":
			chatParts = append(chatParts, ChatContentPart{
				Type: "text",
				Text: part.Text,
			})
		case ContentTypeInputImage, "image_url":
			chatParts = append(chatParts, ChatContentPart{
				Type: "image_url",
				ImageURL: &ChatImageURL{
					URL: part.ImageURL,
				},
			})
		}
	}

	return chatParts, nil
}

// getConversationHistory recursively builds conversation history from a response chain.
func getConversationHistory(resp *Response, store *Store) []ChatMessage {
	var messages []ChatMessage

	// First, get history from previous response
	if resp.PreviousResponseID != nil && *resp.PreviousResponseID != "" && store != nil {
		prevResp, ok := store.Get(*resp.PreviousResponseID)
		if ok {
			messages = append(messages, getConversationHistory(prevResp, store)...)
		}
	}

	// Add this response's output as assistant messages
	for _, item := range resp.Output {
		switch item.Type {
		case ItemTypeMessage:
			msg := ChatMessage{
				Role: item.Role,
			}
			// Extract text content
			var textParts []string
			for _, part := range item.Content {
				if part.Type == ContentTypeOutputText {
					textParts = append(textParts, part.Text)
				}
			}
			msg.Content = strings.Join(textParts, "")
			messages = append(messages, msg)

		case ItemTypeFunctionCall:
			// Add assistant message with tool call
			messages = append(messages, ChatMessage{
				Role: "assistant",
				ToolCalls: []ChatToolCall{{
					ID:   item.CallID,
					Type: "function",
					Function: ChatFunctionCall{
						Name:      item.Name,
						Arguments: item.Arguments,
					},
				}},
			})
		}
	}

	return messages
}

// TransformChatCompletionToResponse converts a chat completion response to a Responses API response.
func TransformChatCompletionToResponse(chatResp *ChatCompletionResponse, respID, model string) *Response {
	resp := NewResponse(respID, model)
	resp.Status = StatusCompleted

	if len(chatResp.Choices) > 0 {
		choice := chatResp.Choices[0]

		// Handle tool calls
		if len(choice.Message.ToolCalls) > 0 {
			for _, tc := range choice.Message.ToolCalls {
				resp.Output = append(resp.Output, OutputItem{
					ID:        GenerateItemID(),
					Type:      ItemTypeFunctionCall,
					Status:    StatusCompleted,
					CallID:    tc.ID,
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				})
			}
		}

		// Handle text content (separately from tool calls, so both can exist in the same response)
		if content, ok := choice.Message.Content.(string); ok && content != "" {
			msgItem := OutputItem{
				ID:     GenerateMessageID(),
				Type:   ItemTypeMessage,
				Role:   "assistant",
				Status: StatusCompleted,
				Content: []ContentPart{{
					Type:        ContentTypeOutputText,
					Text:        content,
					Annotations: []Annotation{},
				}},
			}
			resp.Output = append(resp.Output, msgItem)
			resp.OutputText = content
		} else if contentParts, ok := choice.Message.Content.([]ChatContentPart); ok && len(contentParts) > 0 {
			// Handle multi-part content (e.g., text + images)
			var outputText string
			var contentPartsList []ContentPart

			for _, part := range contentParts {
				var contentPart ContentPart
				switch part.Type {
				case "text":
					contentPart = ContentPart{
						Type:        ContentTypeOutputText,
						Text:        part.Text,
						Annotations: []Annotation{},
					}
					outputText += part.Text
				case "image_url":
					if part.ImageURL != nil {
						contentPart = ContentPart{
							Type:        ContentTypeOutputText,                         // Map to output text for compatibility
							Text:        fmt.Sprintf("[Image: %s]", part.ImageURL.URL), // Include URL reference
							Annotations: []Annotation{},
						}
						// Add image reference to output text
						if outputText != "" {
							outputText += " "
						}
						outputText += fmt.Sprintf("[Image: %s]", part.ImageURL.URL)
					}
				default:
					// Skip unknown content types
					continue
				}
				contentPartsList = append(contentPartsList, contentPart)
			}

			if len(contentPartsList) > 0 {
				msgItem := OutputItem{
					ID:      GenerateMessageID(),
					Type:    ItemTypeMessage,
					Role:    "assistant",
					Status:  StatusCompleted,
					Content: contentPartsList,
				}
				resp.Output = append(resp.Output, msgItem)
				resp.OutputText = outputText
			}
		}
	}

	// Convert usage
	if chatResp.Usage != nil {
		resp.Usage = &Usage{
			InputTokens:  chatResp.Usage.PromptTokens,
			OutputTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:  chatResp.Usage.TotalTokens,
		}
	}

	return resp
}

// MarshalChatCompletionRequest marshals a chat completion request to JSON.
func MarshalChatCompletionRequest(req *ChatCompletionRequest) ([]byte, error) {
	return json.Marshal(req)
}
