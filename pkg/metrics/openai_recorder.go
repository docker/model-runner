package metrics

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/logging"
)

// maximumRecordsPerModel is the maximum number of records that will be stored
// per model.
const maximumRecordsPerModel = 10

// subscriberChannelBuffer is the buffer size for subscriber channels.
const subscriberChannelBuffer = 100

// defaultStreamingErrorCode is the default code for streaming errors.
const defaultStreamingErrorCode = http.StatusBadRequest

// maxMediaDataLength is the maximum length of base64 media data to preserve before truncation.
const maxMediaDataLength = 100

// StreamingError represents an error that occurred during streaming response processing.
// It contains the HTTP status code and additional context about the error.
type StreamingError struct {
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	Type       string `json:"type,omitempty"`
	Details    string `json:"details,omitempty"`
}

// Error implements the error interface for StreamingError.
func (e *StreamingError) Error() string {
	if e.Type != "" {
		return fmt.Sprintf("streaming error (code %d, type %s): %s", e.StatusCode, e.Type, e.Message)
	}
	return fmt.Sprintf("streaming error (code %d): %s", e.StatusCode, e.Message)
}

// GetStatusCode returns the HTTP status code associated with this streaming error.
func (e *StreamingError) GetStatusCode() int {
	return e.StatusCode
}

type responseRecorder struct {
	http.ResponseWriter
	body       *bytes.Buffer
	statusCode int
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
	rr.body.Write(b)
	return rr.ResponseWriter.Write(b)
}

func (rr *responseRecorder) WriteHeader(statusCode int) {
	rr.statusCode = statusCode
	rr.ResponseWriter.WriteHeader(statusCode)
}

func (rr *responseRecorder) Flush() {
	if flusher, ok := rr.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

type RequestResponsePair struct {
	ID         string `json:"id"`
	Model      string `json:"model"`
	Method     string `json:"method"`
	URL        string `json:"url"`
	Request    string `json:"request"`
	Response   string `json:"response,omitempty"`
	Error      string `json:"error,omitempty"`
	Timestamp  int64  `json:"timestamp"`
	StatusCode int    `json:"status_code"`
	UserAgent  string `json:"user_agent,omitempty"`
}

type ModelData struct {
	Config  inference.BackendConfiguration `json:"config"`
	Records []*RequestResponsePair         `json:"records"`
}

type ModelRecordsResponse struct {
	Count int    `json:"count"`
	Model string `json:"model"`
	ModelData
}

type OpenAIRecorder struct {
	log          logging.Logger
	records      map[string]*ModelData // key is model ID
	modelManager *models.Manager       // for resolving model tags to IDs
	m            sync.RWMutex

	// streaming
	subscribers map[string]chan []ModelRecordsResponse
	subMutex    sync.RWMutex
}

func NewOpenAIRecorder(log logging.Logger, modelManager *models.Manager) *OpenAIRecorder {
	return &OpenAIRecorder{
		log:          log,
		modelManager: modelManager,
		records:      make(map[string]*ModelData),
		subscribers:  make(map[string]chan []ModelRecordsResponse),
	}
}

// truncateMediaFields truncates large base64 media data in image_url.url and input_audio.data fields
// to reduce memory usage while preserving request structure information.
func (r *OpenAIRecorder) truncateMediaFields(requestBody []byte) []byte {
	// Try to parse the JSON request
	var requestData map[string]interface{}
	if err := json.Unmarshal(requestBody, &requestData); err != nil {
		// If we can't parse the JSON, return the original body
		return requestBody
	}

	// Look for messages array
	messages, ok := requestData["messages"].([]interface{})
	if !ok {
		// No messages array found, return original
		return requestBody
	}

	// Process each message
	for _, msg := range messages {
		message, ok := msg.(map[string]interface{})
		if !ok {
			continue
		}

		// Look for content array
		content, ok := message["content"].([]interface{})
		if !ok {
			continue
		}

		// Process each content item
		for _, contentItem := range content {
			item, ok := contentItem.(map[string]interface{})
			if !ok {
				continue
			}

			// Handle image_url fields
			if imageURL, exists := item["image_url"].(map[string]interface{}); exists {
				if url, urlExists := imageURL["url"].(string); urlExists {
					imageURL["url"] = r.truncateBase64Data(url)
				}
			}

			// Handle input_audio fields
			if inputAudio, exists := item["input_audio"].(map[string]interface{}); exists {
				if data, dataExists := inputAudio["data"].(string); dataExists {
					inputAudio["data"] = r.truncateBase64Data(data)
				}
			}
		}
	}

	// Marshal back to JSON
	modifiedBody, err := json.Marshal(requestData)
	if err != nil {
		// If marshaling fails, return original body
		return requestBody
	}

	return modifiedBody
}

// truncateBase64Data truncates base64 data strings to a manageable length
func (r *OpenAIRecorder) truncateBase64Data(data string) string {
	if len(data) <= maxMediaDataLength {
		return data
	}

	// Check if it looks like base64 data (data URL or raw base64)
	if strings.HasPrefix(data, "data:") {
		// Handle data URLs like "data:image/jpeg;base64,..."
		parts := strings.SplitN(data, ",", 2)
		if len(parts) == 2 && len(parts[1]) > maxMediaDataLength {
			truncatedChars := len(parts[1]) - maxMediaDataLength
			return fmt.Sprintf("%s,%s...[truncated %d chars]", parts[0], parts[1][:maxMediaDataLength], truncatedChars)
		}
	} else {
		// Handle raw base64 data
		truncatedChars := len(data) - maxMediaDataLength
		return fmt.Sprintf("%s...[truncated %d chars]", data[:maxMediaDataLength], truncatedChars)
	}

	return data
}

func (r *OpenAIRecorder) SetConfigForModel(model string, config *inference.BackendConfiguration) {
	if config == nil {
		r.log.Warnf("SetConfigForModel called with nil config for model %s", model)
		return
	}

	modelID := r.modelManager.ResolveID(model)

	r.m.Lock()
	defer r.m.Unlock()

	if r.records[modelID] == nil {
		r.records[modelID] = &ModelData{
			Records: make([]*RequestResponsePair, 0, 10),
			Config:  inference.BackendConfiguration{},
		}
	}

	r.records[modelID].Config = *config
}

func (r *OpenAIRecorder) RecordRequest(model string, req *http.Request, body []byte) string {
	modelID := r.modelManager.ResolveID(model)

	r.m.Lock()
	defer r.m.Unlock()

	recordID := fmt.Sprintf("%s_%d", modelID, time.Now().UnixNano())

	record := &RequestResponsePair{
		ID:        recordID,
		Model:     model,
		Method:    req.Method,
		URL:       req.URL.Path,
		Request:   string(r.truncateMediaFields(body)),
		Timestamp: time.Now().Unix(),
		UserAgent: req.UserAgent(),
	}

	modelData := r.records[modelID]
	if modelData == nil {
		modelData = &ModelData{
			Records: make([]*RequestResponsePair, 0, maximumRecordsPerModel),
			Config:  inference.BackendConfiguration{},
		}
		r.records[modelID] = modelData
	}

	// Ideally we would use a ring buffer or a linked list for storing records,
	// but we want this data returnable as JSON, so we have to live with this
	// slightly inefficieny memory shuffle. Note that truncating the front of
	// the slice and continually appending would cause the slice's capacity to
	// grow unbounded.
	if len(modelData.Records) == maximumRecordsPerModel {
		copy(
			modelData.Records[:maximumRecordsPerModel-1],
			modelData.Records[1:],
		)
		modelData.Records[maximumRecordsPerModel-1] = record
	} else {
		modelData.Records = append(modelData.Records, record)
	}

	return recordID
}

func (r *OpenAIRecorder) NewResponseRecorder(w http.ResponseWriter) http.ResponseWriter {
	rc := &responseRecorder{
		ResponseWriter: w,
		body:           &bytes.Buffer{},
		statusCode:     0,
	}
	return rc
}

// normalizeErrorToJSON ensures the error content is always valid JSON.
// If the error content is already valid JSON, it is returned as-is.
// If not, it is wrapped in a JSON object with an "error" field.
func (r *OpenAIRecorder) normalizeErrorToJSON(errorContent string) string {
	if errorContent == "" {
		return `{"error": "unknown error"}`
	}

	// Try to parse as JSON to check if it's already valid
	var jsonTest interface{}
	if err := json.Unmarshal([]byte(errorContent), &jsonTest); err == nil {
		// Already valid JSON, return as-is
		return errorContent
	}

	// Not valid JSON, wrap it in a JSON object
	// Escape the content properly for JSON
	escapedContent, err := json.Marshal(errorContent)
	if err != nil {
		// If marshaling fails, create a simple error object
		return `{"error": "Failed to process error content"}`
	}

	return fmt.Sprintf(`{"error": %s}`, string(escapedContent))
}

// handleErrorRecording handles the logic for recording errors and responses based on
// streaming errors and HTTP status codes.
func (r *OpenAIRecorder) handleErrorRecording(record *RequestResponsePair, streamingErr error, response string, statusCode int) {
	if streamingErr != nil {
		record.Error = r.serializeStreamingError(streamingErr)
		record.Response = ""
		return
	}

	if statusCode >= 400 {
		record.Error = r.normalizeErrorToJSON(response)
		record.Response = ""
		return
	}

	// Success case
	record.Response = response
	record.Error = ""
}

// serializeStreamingError handles the serialization of streaming errors.
// It attempts to serialize StreamingError types directly to JSON for rich error structure,
// falling back to normalized JSON for other error types.
func (r *OpenAIRecorder) serializeStreamingError(err error) string {
	var streamingError *StreamingError
	if errors.As(err, &streamingError) {
		if errorJSON, marshalErr := json.Marshal(streamingError); marshalErr == nil {
			return string(errorJSON)
		}
	}
	return r.normalizeErrorToJSON(err.Error())
}

func (r *OpenAIRecorder) RecordResponse(id, model string, rw http.ResponseWriter) {
	rr := rw.(*responseRecorder)

	responseBody := rr.body.String()
	statusCode := rr.statusCode
	if statusCode == 0 {
		// No status code was written (request canceled or failed before response).
		statusCode = http.StatusRequestTimeout
	}

	var response string
	var streamingErr error
	if strings.Contains(responseBody, "data: ") {
		response, streamingErr = r.convertStreamingResponse(responseBody)
	} else {
		response = responseBody
	}

	modelID := r.modelManager.ResolveID(model)

	r.m.Lock()
	defer r.m.Unlock()

	if modelData, exists := r.records[modelID]; exists {
		for _, record := range modelData.Records {
			if record.ID == id {
				record.StatusCode = statusCode
				r.handleErrorRecording(record, streamingErr, response, statusCode)
				// Create ModelRecordsResponse with this single updated record to match
				// what the non-streaming endpoint returns - []ModelRecordsResponse.
				// See getAllRecords and getRecordsByModel.
				modelResponse := []ModelRecordsResponse{{
					Count: 1,
					Model: model,
					ModelData: ModelData{
						Config:  modelData.Config,
						Records: []*RequestResponsePair{record},
					},
				}}
				go r.broadcastToSubscribers(modelResponse)
				return
			}
		}
		r.log.Errorf("Matching request (id=%s) not found for model %s - %d\n%s", id, modelID, statusCode, response)
	} else {
		r.log.Errorf("Model %s not found in records - %d\n%s", modelID, statusCode, response)
	}
}

// convertStreamingResponse converts a streaming response body into a standard JSON response.
// It handles both successful streaming completions and streaming errors.
// If a streaming error is detected, it returns the original streaming body and the error.
// If successful, it reconstructs the final response in standard JSON format.
func (r *OpenAIRecorder) convertStreamingResponse(streamingBody string) (string, error) {
	lines := strings.Split(streamingBody, "\n")
	var contentBuilder strings.Builder
	var reasoningContentBuilder strings.Builder
	var lastChoice, lastChunk map[string]interface{}

	for _, line := range lines {
		// Check for error lines in the streaming format
		if strings.HasPrefix(line, "error: ") {
			errorData := strings.TrimPrefix(line, "error: ")
			var errorObj map[string]interface{}
			if err := json.Unmarshal([]byte(errorData), &errorObj); err == nil {
				// Create a StreamingError with extracted information
				streamingErr := &StreamingError{
					StatusCode: defaultStreamingErrorCode,
					Message:    "streaming error",
				}

				// Extract error code if available
				if code, ok := errorObj["code"].(float64); ok {
					streamingErr.StatusCode = int(code)
				}

				// Extract error message if available
				if message, ok := errorObj["message"].(string); ok {
					streamingErr.Message = message
				}

				// Extract error type if available
				if errorType, ok := errorObj["type"].(string); ok {
					streamingErr.Type = errorType
				}

				// Store original error data as details
				streamingErr.Details = errorData

				// Return the original streaming body for error cases
				return streamingBody, streamingErr
			}
			// If we can't parse the error JSON, create a generic error
			return streamingBody, &StreamingError{
				StatusCode: defaultStreamingErrorCode,
				Message:    "unparseable streaming error",
				Details:    errorData,
			}
		}

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}

			var chunk map[string]interface{}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			lastChunk = chunk

			if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
				if choice, ok := choices[0].(map[string]interface{}); ok {
					lastChoice = choice
					if delta, ok := choice["delta"].(map[string]interface{}); ok {
						if content, ok := delta["content"].(string); ok {
							contentBuilder.WriteString(content)
						}
						if content, ok := delta["reasoning_content"].(string); ok {
							reasoningContentBuilder.WriteString(content)
						}
					}
				}
			}
		}
	}

	if lastChunk == nil {
		return streamingBody, nil
	}

	finalResponse := make(map[string]interface{})

	for key, value := range lastChunk {
		finalResponse[key] = value
	}
	finalResponse["choices"] = []interface{}{lastChoice}

	if choices, ok := finalResponse["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			message := map[string]interface{}{
				"role":    "assistant",
				"content": contentBuilder.String(),
			}
			if reasoningContentBuilder.Len() > 0 {
				message["reasoning_content"] = reasoningContentBuilder.String()
			}
			choice["message"] = message
			delete(choice, "delta")

			if _, ok := choice["finish_reason"]; !ok {
				choice["finish_reason"] = "stop"
			}
		}
	}

	finalResponse["object"] = "chat.completion"

	jsonResult, err := json.Marshal(finalResponse)
	if err != nil {
		return streamingBody, nil
	}

	return string(jsonResult), nil
}

func (r *OpenAIRecorder) GetRecordsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		acceptHeader := req.Header.Get("Accept")

		// Check if client wants Server-Sent Events
		if acceptHeader == "text/event-stream" {
			r.handleStreamingRequests(w, req)
			return
		}

		// Default to JSON response
		r.handleJSONRequests(w, req)
	}
}

func (r *OpenAIRecorder) handleJSONRequests(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	model := req.URL.Query().Get("model")

	if model == "" {
		// Retrieve all records for all models.
		allRecords := r.getAllRecords()
		if allRecords == nil {
			allRecords = []ModelRecordsResponse{}
		}
		if err := json.NewEncoder(w).Encode(allRecords); err != nil {
			http.Error(w, fmt.Sprintf("Failed to encode all records: %v", err),
				http.StatusInternalServerError)
			return
		}
	} else {
		// Retrieve records for the specified model.
		records := r.getRecordsByModel(model)
		if records == nil {
			records = []ModelRecordsResponse{}
		}
		if err := json.NewEncoder(w).Encode(records); err != nil {
			http.Error(w, fmt.Sprintf("Failed to encode records for model '%s': %v", model, err),
				http.StatusInternalServerError)
			return
		}
	}
}

func (r *OpenAIRecorder) handleStreamingRequests(w http.ResponseWriter, req *http.Request) {
	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Create subscriber channel.
	subscriberID := fmt.Sprintf("sub_%d", time.Now().UnixNano())
	ch := make(chan []ModelRecordsResponse, subscriberChannelBuffer)

	// Register subscriber.
	r.subMutex.Lock()
	r.subscribers[subscriberID] = ch
	r.subMutex.Unlock()

	// Clean up on disconnect.
	defer func() {
		r.subMutex.Lock()
		delete(r.subscribers, subscriberID)
		close(ch)
		r.subMutex.Unlock()
	}()

	// Optional: Send existing records first.
	model := req.URL.Query().Get("model")
	if includeExisting := req.URL.Query().Get("include_existing"); includeExisting == "true" {
		r.sendExistingRecords(w, model)
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send heartbeat to establish connection.
	if _, err := fmt.Fprintf(w, "event: connected\ndata: {\"status\": \"connected\"}\n\n"); err != nil {
		r.log.Errorf("Failed to write connected event to response: %v", err)
	}
	flusher.Flush()

	for {
		select {
		case modelRecords, ok := <-ch:
			if !ok {
				return
			}

			// Filter by model if specified.
			// modelRecords is assumed to have size 1 because that's how we call broadcastToSubscribers.
			// We do this so we don't need to query a 2nd time for the model config.
			if model != "" && len(modelRecords) > 0 && modelRecords[0].Model != model {
				continue
			}

			// Send as SSE event.
			jsonData, err := json.Marshal(modelRecords)
			if err != nil {
				r.log.Errorf("Failed to marshal record for streaming: %v", err)
				errorMsg := fmt.Sprintf(`{"error": "Failed to marshal record: %v"}`, err)
				if _, writeErr := fmt.Fprintf(w, "event: error\ndata: %s\n\n", errorMsg); writeErr != nil {
					r.log.Errorf("Failed to write error event to response: %v", writeErr)
				}
				flusher.Flush()
				continue
			}

			if _, err := fmt.Fprintf(w, "event: new_request\ndata: %s\n\n", jsonData); err != nil {
				r.log.Errorf("Failed to write new_request event to response: %v", err)
			}
			flusher.Flush()

		case <-req.Context().Done():
			// Client disconnected.
			return
		}
	}
}

func (r *OpenAIRecorder) getAllRecords() []ModelRecordsResponse {
	r.m.RLock()
	defer r.m.RUnlock()

	if len(r.records) == 0 {
		return nil
	}

	result := make([]ModelRecordsResponse, 0, len(r.records))

	for modelID, modelData := range r.records {
		result = append(result, ModelRecordsResponse{
			Count: len(modelData.Records),
			Model: modelID,
			ModelData: ModelData{
				Config:  modelData.Config,
				Records: modelData.Records,
			},
		})
	}

	return result
}

func (r *OpenAIRecorder) getRecordsByModel(model string) []ModelRecordsResponse {
	modelID := r.modelManager.ResolveID(model)

	r.m.RLock()
	defer r.m.RUnlock()

	if modelData, exists := r.records[modelID]; exists {
		return []ModelRecordsResponse{{
			Count: len(modelData.Records),
			Model: modelID,
			ModelData: ModelData{
				Config:  modelData.Config,
				Records: modelData.Records,
			},
		}}
	}

	return nil
}

func (r *OpenAIRecorder) broadcastToSubscribers(modelResponses []ModelRecordsResponse) {
	r.subMutex.RLock()
	defer r.subMutex.RUnlock()

	for _, ch := range r.subscribers {
		select {
		case ch <- modelResponses:
		default:
			// The channel is full, skip this subscriber.
		}
	}
}

func (r *OpenAIRecorder) sendExistingRecords(w http.ResponseWriter, model string) {
	var records []ModelRecordsResponse

	if model == "" {
		records = r.getAllRecords()
	} else {
		records = r.getRecordsByModel(model)
	}

	// Send each individual request-response pair as a separate event.
	for _, modelRecord := range records {
		for _, requestRecord := range modelRecord.Records {
			// Create a ModelRecordsResponse with a single record to match
			// what the non-streaming endpoint returns - []ModelRecordsResponse.
			// See getAllRecords and getRecordsByModel.
			singleRecord := []ModelRecordsResponse{{
				Count: 1,
				Model: modelRecord.Model,
				ModelData: ModelData{
					Config:  modelRecord.Config,
					Records: []*RequestResponsePair{requestRecord},
				},
			}}
			jsonData, err := json.Marshal(singleRecord)
			if err != nil {
				r.log.Errorf("Failed to marshal existing record for streaming: %v", err)
				errorMsg := fmt.Sprintf(`{"error": "Failed to marshal existing record: %v"}`, err)
				if _, writeErr := fmt.Fprintf(w, "event: error\ndata: %s\n\n", errorMsg); writeErr != nil {
					r.log.Errorf("Failed to write error event to response: %v", writeErr)
				}
			} else {
				if _, writeErr := fmt.Fprintf(w, "event: existing_request\ndata: %s\n\n", jsonData); writeErr != nil {
					r.log.Errorf("Failed to write existing_request event to response: %v", writeErr)
				}
			}
		}
	}
}

func (r *OpenAIRecorder) RemoveModel(model string) {
	modelID := r.modelManager.ResolveID(model)

	r.m.Lock()
	defer r.m.Unlock()

	if _, exists := r.records[modelID]; exists {
		delete(r.records, modelID)
		r.log.Infof("Removed records for model: %s", modelID)
	} else {
		r.log.Warnf("No records found for model: %s", modelID)
	}
}
