package responses

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

// mockSchedulerHTTP is a mock scheduler that returns predefined responses.
type mockSchedulerHTTP struct {
	response     string
	statusCode   int
	streaming    bool
	streamChunks []string
}

func (m *mockSchedulerHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if m.streaming {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, chunk := range m.streamChunks {
			w.Write([]byte(chunk))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(m.statusCode)
	w.Write([]byte(m.response))
}

func newTestHandler(mock *mockSchedulerHTTP) *HTTPHandler {
	log := logrus.New()
	log.SetOutput(io.Discard)
	return NewHTTPHandler(log, mock, nil)
}

func TestHandler_CreateResponse_NonStreaming(t *testing.T) {
	mock := &mockSchedulerHTTP{
		statusCode: http.StatusOK,
		response: `{
			"id": "chatcmpl-123",
			"object": "chat.completion",
			"created": 1234567890,
			"model": "gpt-4",
			"choices": [
				{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "Hello! How can I help you?"
					},
					"finish_reason": "stop"
				}
			],
			"usage": {
				"prompt_tokens": 10,
				"completion_tokens": 7,
				"total_tokens": 17
			}
		}`,
	}

	handler := newTestHandler(mock)

	reqBody := `{
		"model": "gpt-4",
		"input": "Hello"
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handleCreate(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.Object != "response" {
		t.Errorf("object = %s, want response", result.Object)
	}
	if result.Model != "gpt-4" {
		t.Errorf("model = %s, want gpt-4", result.Model)
	}
	if result.Status != StatusCompleted {
		t.Errorf("status = %s, want %s", result.Status, StatusCompleted)
	}
	if result.OutputText != "Hello! How can I help you?" {
		t.Errorf("output_text = %s, want Hello! How can I help you?", result.OutputText)
	}
	if !strings.HasPrefix(result.ID, "resp_") {
		t.Errorf("id should start with resp_, got %s", result.ID)
	}
}

func TestHandler_CreateResponse_MissingModel(t *testing.T) {
	mock := &mockSchedulerHTTP{}
	handler := newTestHandler(mock)

	reqBody := `{"input": "Hello"}`

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handleCreate(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	var errResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&errResp)

	if errResp["error"] == nil {
		t.Error("expected error in response")
	}
}

func TestHandler_CreateResponse_InvalidJSON(t *testing.T) {
	mock := &mockSchedulerHTTP{}
	handler := newTestHandler(mock)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{invalid`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handleCreate(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestHandler_GetResponse(t *testing.T) {
	mock := &mockSchedulerHTTP{}
	handler := newTestHandler(mock)

	// First, store a response
	testResp := NewResponse("resp_test123", "gpt-4")
	testResp.Status = StatusCompleted
	testResp.OutputText = "Test output"
	handler.store.Save(testResp)

	// Now retrieve it
	req := httptest.NewRequest(http.MethodGet, "/v1/responses/resp_test123", http.NoBody)
	req.SetPathValue("id", "resp_test123")
	w := httptest.NewRecorder()

	handler.handleGet(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.ID != "resp_test123" {
		t.Errorf("id = %s, want resp_test123", result.ID)
	}
	if result.OutputText != "Test output" {
		t.Errorf("output_text = %s, want Test output", result.OutputText)
	}
}

func TestHandler_GetResponse_NotFound(t *testing.T) {
	mock := &mockSchedulerHTTP{}
	handler := newTestHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/v1/responses/nonexistent", http.NoBody)
	req.SetPathValue("id", "nonexistent")
	w := httptest.NewRecorder()

	handler.handleGet(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestHandler_DeleteResponse(t *testing.T) {
	mock := &mockSchedulerHTTP{}
	handler := newTestHandler(mock)

	// First, store a response
	testResp := NewResponse("resp_test123", "gpt-4")
	handler.store.Save(testResp)

	// Delete it
	req := httptest.NewRequest(http.MethodDelete, "/v1/responses/resp_test123", http.NoBody)
	req.SetPathValue("id", "resp_test123")
	w := httptest.NewRecorder()

	handler.handleDelete(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Verify it's deleted
	_, ok := handler.store.Get("resp_test123")
	if ok {
		t.Error("expected response to be deleted")
	}
}

func TestHandler_DeleteResponse_NotFound(t *testing.T) {
	mock := &mockSchedulerHTTP{}
	handler := newTestHandler(mock)

	req := httptest.NewRequest(http.MethodDelete, "/v1/responses/nonexistent", http.NoBody)
	req.SetPathValue("id", "nonexistent")
	w := httptest.NewRecorder()

	handler.handleDelete(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestHandler_CreateResponse_WithPreviousResponse(t *testing.T) {
	mock := &mockSchedulerHTTP{
		statusCode: http.StatusOK,
		response: `{
			"id": "chatcmpl-456",
			"object": "chat.completion",
			"created": 1234567891,
			"model": "gpt-4",
			"choices": [
				{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "I'm doing well, thanks!"
					},
					"finish_reason": "stop"
				}
			]
		}`,
	}

	handler := newTestHandler(mock)

	// Create a previous response
	prevResp := NewResponse("resp_prev123", "gpt-4")
	prevResp.Status = StatusCompleted
	prevResp.Output = []OutputItem{
		{
			ID:   "msg_1",
			Type: ItemTypeMessage,
			Role: "assistant",
			Content: []ContentPart{
				{Type: ContentTypeOutputText, Text: "Hello!"},
			},
		},
	}
	handler.store.Save(prevResp)

	// Create new request chained to previous
	reqBody := `{
		"model": "gpt-4",
		"input": "How are you?",
		"previous_response_id": "resp_prev123"
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handleCreate(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d, body: %s", resp.StatusCode, http.StatusOK, body)
	}

	var result Response
	json.NewDecoder(resp.Body).Decode(&result)

	if result.PreviousResponseID == nil || *result.PreviousResponseID != "resp_prev123" {
		t.Errorf("previous_response_id = %v, want resp_prev123", result.PreviousResponseID)
	}
}

func TestHandler_CreateResponse_UpstreamError(t *testing.T) {
	mock := &mockSchedulerHTTP{
		statusCode: http.StatusInternalServerError,
		response: `{
			"error": {
				"message": "Model overloaded",
				"type": "server_error",
				"code": "model_overloaded"
			}
		}`,
	}

	handler := newTestHandler(mock)

	reqBody := `{
		"model": "gpt-4",
		"input": "Hello"
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handleCreate(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}

	var result Response
	json.NewDecoder(resp.Body).Decode(&result)

	if result.Status != StatusFailed {
		t.Errorf("status = %s, want %s", result.Status, StatusFailed)
	}
	if result.Error == nil {
		t.Error("expected error to be set")
	}
}

func TestHandler_CreateResponse_UpstreamError_NonJSONBody(t *testing.T) {
	mock := &mockSchedulerHTTP{
		statusCode: http.StatusInternalServerError,
		// non-JSON / malformed body to exercise the fallback branch in handleNonStreaming
		response: "upstream exploded in a non-json way",
	}

	handler := newTestHandler(mock)

	reqBody := `{
		"model": "gpt-4",
		"input": "Hello"
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handleCreate(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}

	var result Response
	json.NewDecoder(resp.Body).Decode(&result)

	// Assert: non-streaming error handling falls back correctly
	if result.Status != StatusFailed {
		t.Errorf("status = %s, want %s", result.Status, StatusFailed)
	}

	if result.Error == nil {
		t.Fatalf("expected error, got nil")
	}

	if result.Error.Code != "upstream_error" {
		t.Errorf("error.code = %v, want upstream_error", result.Error.Code)
	}

	if !strings.Contains(result.Error.Message, "upstream exploded in a non-json way") {
		t.Errorf("error.message = %q, want to contain raw upstream body", result.Error.Message)
	}
}

func TestHandler_CreateResponse_Streaming(t *testing.T) {
	// Mock streaming response
	mock := &mockSchedulerHTTP{
		streaming: true,
		streamChunks: []string{
			"data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n",
			"data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n",
			"data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"!\"},\"finish_reason\":null}]}\n\n",
			"data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n",
			"data: [DONE]\n\n",
		},
	}

	handler := newTestHandler(mock)

	reqBody := `{
		"model": "gpt-4",
		"input": "Hello",
		"stream": true
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handleCreate(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Check content type is SSE
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		t.Errorf("Content-Type = %s, want text/event-stream", contentType)
	}

	// Read all body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	bodyStr := string(body)

	// Verify we got the expected events
	if !strings.Contains(bodyStr, "response.created") {
		t.Error("expected response.created event")
	}
	if !strings.Contains(bodyStr, "response.output_text.delta") {
		t.Error("expected response.output_text.delta event")
	}
	if !strings.Contains(bodyStr, "response.completed") {
		t.Error("expected response.completed event")
	}
}

func TestHandler_CreateResponse_WithTools(t *testing.T) {
	// Mock response with tool call
	mock := &mockSchedulerHTTP{
		statusCode: http.StatusOK,
		response: `{
			"id": "chatcmpl-123",
			"object": "chat.completion",
			"created": 1234567890,
			"model": "gpt-4",
			"choices": [
				{
					"index": 0,
					"message": {
						"role": "assistant",
						"tool_calls": [
							{
								"id": "call_abc123",
								"type": "function",
								"function": {
									"name": "get_weather",
									"arguments": "{\"location\": \"San Francisco\"}"
								}
							}
						]
					},
					"finish_reason": "tool_calls"
				}
			]
		}`,
	}

	handler := newTestHandler(mock)

	reqBody := `{
		"model": "gpt-4",
		"input": "What's the weather in San Francisco?",
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "get_weather",
					"description": "Get weather information",
					"parameters": {
						"type": "object",
						"properties": {
							"location": {"type": "string"}
						}
					}
				}
			}
		]
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handleCreate(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d, body: %s", resp.StatusCode, http.StatusOK, body)
	}

	var result Response
	json.NewDecoder(resp.Body).Decode(&result)

	if len(result.Output) == 0 {
		t.Fatal("expected output items")
	}

	// Find the function call item
	var funcCall *OutputItem
	for i := range result.Output {
		if result.Output[i].Type == ItemTypeFunctionCall {
			funcCall = &result.Output[i]
			break
		}
	}

	if funcCall == nil {
		t.Fatal("expected function call in output")
	}

	if funcCall.Name != "get_weather" {
		t.Errorf("function name = %s, want get_weather", funcCall.Name)
	}
	if funcCall.CallID != "call_abc123" {
		t.Errorf("call_id = %s, want call_abc123", funcCall.CallID)
	}
}

// Test that stored responses persist across requests
func TestHandler_ResponsePersistence(t *testing.T) {
	mock := &mockSchedulerHTTP{
		statusCode: http.StatusOK,
		response: `{
			"id": "chatcmpl-123",
			"choices": [
				{
					"message": {
						"role": "assistant",
						"content": "Hello!"
					}
				}
			]
		}`,
	}

	handler := newTestHandler(mock)

	// Create a response
	reqBody := `{"model": "gpt-4", "input": "Hi"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handleCreate(w, req)

	var createResult Response
	json.NewDecoder(w.Result().Body).Decode(&createResult)

	// Retrieve it
	req2 := httptest.NewRequest(http.MethodGet, "/v1/responses/"+createResult.ID, http.NoBody)
	req2.SetPathValue("id", createResult.ID)
	w2 := httptest.NewRecorder()

	handler.handleGet(w2, req2)

	var getResult Response
	json.NewDecoder(w2.Result().Body).Decode(&getResult)

	if getResult.ID != createResult.ID {
		t.Errorf("IDs don't match: %s vs %s", getResult.ID, createResult.ID)
	}
}

// Test that streaming responses are properly persisted in the store
func TestHandler_CreateResponse_Streaming_Persistence(t *testing.T) {
	// Mock streaming response
	mock := &mockSchedulerHTTP{
		streaming: true,
		streamChunks: []string{
			"data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n",
			"data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n",
			"data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"!\"},\"finish_reason\":null}]}\n\n",
			"data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n",
			"data: [DONE]\n\n",
		},
	}

	handler := newTestHandler(mock)

	reqBody := `{
		"model": "gpt-4",
		"input": "Hello",
		"stream": true
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handleCreate(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Verify that the StreamingResponseWriter persisted a coherent Response in the store
	memStore := handler.store

	if memStore.Count() != 1 {
		t.Fatalf("expected exactly one response in store, got %d", memStore.Count())
	}

	// Get the response ID from the store
	responseIDs := memStore.GetResponseIDs()
	if len(responseIDs) != 1 {
		t.Fatalf("expected exactly one response ID in store, got %d", len(responseIDs))
	}

	// Retrieve the response using the public API
	persistedResp, ok := memStore.Get(responseIDs[0])
	if !ok {
		t.Fatal("expected to retrieve persisted Response from store")
	}

	// Status should be completed after streaming finishes
	if persistedResp.Status != StatusCompleted {
		t.Errorf("persisted response status = %s, want %s", persistedResp.Status, StatusCompleted)
	}

	// OutputText should match concatenated streamed chunks: "Hello" + "!" => "Hello!"
	if persistedResp.OutputText != "Hello!" {
		t.Errorf("persisted response OutputText = %q, want %q", persistedResp.OutputText, "Hello!")
	}

	// There should be at least one OutputItem whose message content matches "Hello!"
	found := false
	for _, item := range persistedResp.Output {
		if item.Type != ItemTypeMessage {
			continue
		}
		// Check if the message contains the expected text
		for _, contentPart := range item.Content {
			if contentPart.Type == ContentTypeOutputText && contentPart.Text == "Hello!" {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Errorf("expected an OutputItem message with text %q in persisted response", "Hello!")
	}
}

// Benchmark for response creation
func BenchmarkHandler_CreateResponse(b *testing.B) {
	mock := &mockSchedulerHTTP{
		statusCode: http.StatusOK,
		response: `{
			"id": "chatcmpl-123",
			"choices": [
				{
					"message": {
						"role": "assistant",
						"content": "Hello!"
					}
				}
			]
		}`,
	}

	handler := newTestHandler(mock)
	reqBody := []byte(`{"model": "gpt-4", "input": "Hello"}`)

	for b.Loop() {
		req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.handleCreate(w, req)
	}
}
