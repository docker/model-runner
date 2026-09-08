package responses

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTransformRequestToChatCompletion_SimpleText(t *testing.T) {
	req := &CreateRequest{
		Model: "gpt-4",
		Input: json.RawMessage(`"Hello, how are you?"`),
	}

	chatReq, err := TransformRequestToChatCompletion(req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if chatReq.Model != "gpt-4" {
		t.Errorf("got model %s, want gpt-4", chatReq.Model)
	}

	if len(chatReq.Messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(chatReq.Messages))
	}

	if chatReq.Messages[0].Role != "user" {
		t.Errorf("got role %s, want user", chatReq.Messages[0].Role)
	}

	content, ok := chatReq.Messages[0].Content.(string)
	if !ok {
		t.Fatalf("expected string content")
	}
	if content != "Hello, how are you?" {
		t.Errorf("got content %s, want Hello, how are you?", content)
	}
}

func TestTransformRequestToChatCompletion_WithInstructions(t *testing.T) {
	req := &CreateRequest{
		Model:        "gpt-4",
		Input:        json.RawMessage(`"Tell me a joke"`),
		Instructions: "You are a helpful assistant.",
	}

	chatReq, err := TransformRequestToChatCompletion(req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(chatReq.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(chatReq.Messages))
	}

	// First message should be system
	if chatReq.Messages[0].Role != "system" {
		t.Errorf("first message role = %s, want system", chatReq.Messages[0].Role)
	}
	if chatReq.Messages[0].Content != "You are a helpful assistant." {
		t.Errorf("system content = %v, want You are a helpful assistant.", chatReq.Messages[0].Content)
	}

	// Second message should be user
	if chatReq.Messages[1].Role != "user" {
		t.Errorf("second message role = %s, want user", chatReq.Messages[1].Role)
	}
}

func TestTransformRequestToChatCompletion_MessageArray(t *testing.T) {
	input := `[
		{"role": "user", "content": "Hello"},
		{"role": "assistant", "content": "Hi there!"},
		{"role": "user", "content": "How are you?"}
	]`

	req := &CreateRequest{
		Model: "gpt-4",
		Input: json.RawMessage(input),
	}

	chatReq, err := TransformRequestToChatCompletion(req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(chatReq.Messages) != 3 {
		t.Fatalf("got %d messages, want 3", len(chatReq.Messages))
	}

	expectedRoles := []string{"user", "assistant", "user"}
	for i, msg := range chatReq.Messages {
		if msg.Role != expectedRoles[i] {
			t.Errorf("message %d role = %s, want %s", i, msg.Role, expectedRoles[i])
		}
	}
}

func TestTransformRequestToChatCompletion_MessageArrayWithMultiPartContent(t *testing.T) {
	input := `[
		{
			"role": "user",
			"content": [
				{
					"type": "input_text",
					"text": "Hi"
				},
				{
					"type": "input_image",
					"image_url": "https://example.com/image.png"
				}
			]
		}
	]`

	req := &CreateRequest{
		Model: "gpt-4",
		Input: json.RawMessage(input),
	}

	chatReq, err := TransformRequestToChatCompletion(req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(chatReq.Messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(chatReq.Messages))
	}

	msg := chatReq.Messages[0]
	if msg.Role != "user" {
		t.Fatalf("message role = %s, want user", msg.Role)
	}

	contentParts, ok := msg.Content.([]ChatContentPart)
	if !ok {
		t.Fatalf("message content has type %T, want []ChatContentPart", msg.Content)
	}

	if len(contentParts) != 2 {
		t.Fatalf("got %d content parts, want 2", len(contentParts))
	}

	if contentParts[0].Type != "text" {
		t.Errorf("first content part type = %s, want text", contentParts[0].Type)
	}
	if contentParts[0].Text != "Hi" {
		t.Errorf("first content part text = %q, want %q", contentParts[0].Text, "Hi")
	}

	if contentParts[1].Type != "image_url" {
		t.Errorf("second content part type = %s, want image_url", contentParts[1].Type)
	}
	if contentParts[1].ImageURL == nil || contentParts[1].ImageURL.URL != "https://example.com/image.png" {
		t.Errorf("second content part image_url = %v, want https://example.com/image.png", contentParts[1].ImageURL)
	}
}

func TestTransformRequestToChatCompletion_WithTools(t *testing.T) {
	req := &CreateRequest{
		Model: "gpt-4",
		Input: json.RawMessage(`"What's the weather?"`),
		Tools: []Tool{
			{
				Type: "function",
				Function: &FunctionDef{
					Name:        "get_weather",
					Description: "Get the weather",
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"location": map[string]interface{}{
								"type": "string",
							},
						},
					},
				},
			},
		},
	}

	chatReq, err := TransformRequestToChatCompletion(req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(chatReq.Tools) != 1 {
		t.Fatalf("got %d tools, want 1", len(chatReq.Tools))
	}

	if chatReq.Tools[0].Type != "function" {
		t.Errorf("tool type = %s, want function", chatReq.Tools[0].Type)
	}
	if chatReq.Tools[0].Function.Name != "get_weather" {
		t.Errorf("function name = %s, want get_weather", chatReq.Tools[0].Function.Name)
	}
}

func TestTransformRequestToChatCompletion_FunctionCallOutput(t *testing.T) {
	input := `[
		{"type": "function_call_output", "call_id": "call_123", "output": "{\"temperature\": 72}"}
	]`

	req := &CreateRequest{
		Model: "gpt-4",
		Input: json.RawMessage(input),
	}

	chatReq, err := TransformRequestToChatCompletion(req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(chatReq.Messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(chatReq.Messages))
	}

	if chatReq.Messages[0].Role != "tool" {
		t.Errorf("role = %s, want tool", chatReq.Messages[0].Role)
	}
	if chatReq.Messages[0].ToolCallID != "call_123" {
		t.Errorf("tool_call_id = %s, want call_123", chatReq.Messages[0].ToolCallID)
	}
}

func TestTransformRequestToChatCompletion_PreviousResponseNotFound(t *testing.T) {
	store := NewStore(DefaultTTL)
	t.Cleanup(store.Close)

	req := &CreateRequest{
		Model:              "gpt-4",
		Input:              json.RawMessage(`"Hello"`),
		PreviousResponseID: "resp_missing",
	}

	_, err := TransformRequestToChatCompletion(req, store)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), `previous_response_id "resp_missing" was not found`) {
		t.Fatalf("error = %q, want previous_response_id not found", err.Error())
	}
}

func TestTransformRequestToChatCompletion_PreviousResponseWithoutStore(t *testing.T) {
	req := &CreateRequest{
		Model:              "gpt-4",
		Input:              json.RawMessage(`"Hello"`),
		PreviousResponseID: "resp_missing",
	}

	_, err := TransformRequestToChatCompletion(req, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "response storage is unavailable") {
		t.Fatalf("error = %q, want storage unavailable", err.Error())
	}
}

func TestTransformRequestToChatCompletion_WithParameters(t *testing.T) {
	temp := 0.7
	topP := 0.9
	maxTokens := 100

	req := &CreateRequest{
		Model:           "gpt-4",
		Input:           json.RawMessage(`"Test"`),
		Temperature:     &temp,
		TopP:            &topP,
		MaxOutputTokens: &maxTokens,
		Stream:          true,
	}

	chatReq, err := TransformRequestToChatCompletion(req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if chatReq.Temperature == nil || *chatReq.Temperature != 0.7 {
		t.Errorf("temperature = %v, want 0.7", chatReq.Temperature)
	}
	if chatReq.TopP == nil || *chatReq.TopP != 0.9 {
		t.Errorf("top_p = %v, want 0.9", chatReq.TopP)
	}
	if chatReq.MaxTokens == nil || *chatReq.MaxTokens != 100 {
		t.Errorf("max_tokens = %v, want 100", chatReq.MaxTokens)
	}
	if !chatReq.Stream {
		t.Error("stream should be true")
	}
}

func TestTransformRequestToChatCompletion_TextFormatJSONSchema(t *testing.T) {
	strict := true
	schema := json.RawMessage(`{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}`)
	req := &CreateRequest{
		Model: "gpt-4",
		Input: json.RawMessage(`"Answer in JSON"`),
		Text: &ResponseTextConfig{
			Format: &ResponseTextFormat{
				Type:        "json_schema",
				Name:        "answer_prompt",
				Description: "Answer payload",
				Schema:      schema,
				Strict:      &strict,
			},
		},
	}

	chatReq, err := TransformRequestToChatCompletion(req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if chatReq.ResponseFormat == nil {
		t.Fatal("response_format is nil")
	}
	if chatReq.ResponseFormat.Type != "json_schema" {
		t.Errorf("response_format.type = %s, want json_schema", chatReq.ResponseFormat.Type)
	}
	if chatReq.ResponseFormat.JSONSchema == nil {
		t.Fatal("response_format.json_schema is nil")
	}
	if chatReq.ResponseFormat.JSONSchema.Name != "answer_prompt" {
		t.Errorf("json_schema.name = %s, want answer_prompt", chatReq.ResponseFormat.JSONSchema.Name)
	}
	if chatReq.ResponseFormat.JSONSchema.Description != "Answer payload" {
		t.Errorf("json_schema.description = %s, want Answer payload", chatReq.ResponseFormat.JSONSchema.Description)
	}
	if string(chatReq.ResponseFormat.JSONSchema.Schema) != string(schema) {
		t.Errorf("json_schema.schema = %s, want %s", chatReq.ResponseFormat.JSONSchema.Schema, schema)
	}
	if chatReq.ResponseFormat.JSONSchema.Strict == nil || !*chatReq.ResponseFormat.JSONSchema.Strict {
		t.Errorf("json_schema.strict = %v, want true", chatReq.ResponseFormat.JSONSchema.Strict)
	}
}

func TestTransformRequestToChatCompletion_TextFormatJSONObject(t *testing.T) {
	req := &CreateRequest{
		Model: "gpt-4",
		Input: json.RawMessage(`"Answer with a JSON object"`),
		Text: &ResponseTextConfig{
			Format: &ResponseTextFormat{
				Type: "json_object",
			},
		},
	}

	chatReq, err := TransformRequestToChatCompletion(req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chatReq.ResponseFormat == nil {
		t.Fatal("response_format is nil")
	}
	if chatReq.ResponseFormat.Type != "json_object" {
		t.Errorf("response_format.type = %s, want json_object", chatReq.ResponseFormat.Type)
	}
	if chatReq.ResponseFormat.JSONSchema != nil {
		t.Errorf("response_format.json_schema = %v, want nil", chatReq.ResponseFormat.JSONSchema)
	}
}

func TestTransformRequestToChatCompletion_TextFormatTextOmitted(t *testing.T) {
	req := &CreateRequest{
		Model: "gpt-4",
		Input: json.RawMessage(`"Hello"`),
		Text: &ResponseTextConfig{
			Format: &ResponseTextFormat{
				Type: "text",
			},
		},
	}

	chatReq, err := TransformRequestToChatCompletion(req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chatReq.ResponseFormat != nil {
		t.Errorf("response_format = %v, want nil", chatReq.ResponseFormat)
	}
}

func TestTransformRequestToChatCompletion_TextFormatValidation(t *testing.T) {
	tests := []struct {
		name    string
		format  ResponseTextFormat
		wantErr string
	}{
		{
			name:    "invalid type",
			format:  ResponseTextFormat{Type: "xml"},
			wantErr: "unsupported text.format.type",
		},
		{
			name:    "json schema requires name",
			format:  ResponseTextFormat{Type: "json_schema", Schema: json.RawMessage(`{"type":"object"}`)},
			wantErr: "text.format.name is required",
		},
		{
			name:    "json schema validates name",
			format:  ResponseTextFormat{Type: "json_schema", Name: "not valid", Schema: json.RawMessage(`{"type":"object"}`)},
			wantErr: "text.format.name must contain",
		},
		{
			name:    "json schema requires schema",
			format:  ResponseTextFormat{Type: "json_schema", Name: "answer"},
			wantErr: "text.format.schema is required",
		},
		{
			name:    "json schema must be valid JSON",
			format:  ResponseTextFormat{Type: "json_schema", Name: "answer", Schema: json.RawMessage(`{invalid`)},
			wantErr: "text.format.schema must be a valid JSON object",
		},
		{
			name:    "json schema must be object",
			format:  ResponseTextFormat{Type: "json_schema", Name: "answer", Schema: json.RawMessage(`[]`)},
			wantErr: "text.format.schema must be a JSON object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &CreateRequest{
				Model: "gpt-4",
				Input: json.RawMessage(`"Hello"`),
				Text: &ResponseTextConfig{
					Format: &tt.format,
				},
			}

			_, err := TransformRequestToChatCompletion(req, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestTransformChatCompletionToResponse_TextContent(t *testing.T) {
	chatResp := &ChatCompletionResponse{
		ID:      "chatcmpl-123",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "gpt-4",
		Choices: []ChatChoice{
			{
				Index: 0,
				Message: ChatMessage{
					Role:    "assistant",
					Content: "Hello! How can I help you today?",
				},
				FinishReason: "stop",
			},
		},
		Usage: &ChatUsage{
			PromptTokens:     10,
			CompletionTokens: 8,
			TotalTokens:      18,
		},
	}

	resp := TransformChatCompletionToResponse(chatResp, "resp_test123", "gpt-4")

	if resp.ID != "resp_test123" {
		t.Errorf("ID = %s, want resp_test123", resp.ID)
	}
	if resp.Model != "gpt-4" {
		t.Errorf("Model = %s, want gpt-4", resp.Model)
	}
	if resp.Status != StatusCompleted {
		t.Errorf("Status = %s, want %s", resp.Status, StatusCompleted)
	}
	if resp.OutputText != "Hello! How can I help you today?" {
		t.Errorf("OutputText = %s, want Hello! How can I help you today?", resp.OutputText)
	}

	if len(resp.Output) != 1 {
		t.Fatalf("got %d output items, want 1", len(resp.Output))
	}

	if resp.Output[0].Type != ItemTypeMessage {
		t.Errorf("output type = %s, want %s", resp.Output[0].Type, ItemTypeMessage)
	}
	if resp.Output[0].Role != "assistant" {
		t.Errorf("output role = %s, want assistant", resp.Output[0].Role)
	}

	if resp.Usage == nil {
		t.Fatal("expected usage to be set")
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("input_tokens = %d, want 10", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 8 {
		t.Errorf("output_tokens = %d, want 8", resp.Usage.OutputTokens)
	}
}

func TestTransformChatCompletionToResponse_ToolCalls(t *testing.T) {
	chatResp := &ChatCompletionResponse{
		ID:      "chatcmpl-123",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "gpt-4",
		Choices: []ChatChoice{
			{
				Index: 0,
				Message: ChatMessage{
					Role: "assistant",
					ToolCalls: []ChatToolCall{
						{
							ID:   "call_abc123",
							Type: "function",
							Function: ChatFunctionCall{
								Name:      "get_weather",
								Arguments: `{"location": "San Francisco"}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
	}

	resp := TransformChatCompletionToResponse(chatResp, "resp_test123", "gpt-4")

	if len(resp.Output) != 1 {
		t.Fatalf("got %d output items, want 1", len(resp.Output))
	}

	if resp.Output[0].Type != ItemTypeFunctionCall {
		t.Errorf("output type = %s, want %s", resp.Output[0].Type, ItemTypeFunctionCall)
	}
	if resp.Output[0].CallID != "call_abc123" {
		t.Errorf("call_id = %s, want call_abc123", resp.Output[0].CallID)
	}
	if resp.Output[0].Name != "get_weather" {
		t.Errorf("name = %s, want get_weather", resp.Output[0].Name)
	}
	if resp.Output[0].Arguments != `{"location": "San Francisco"}` {
		t.Errorf("arguments = %s, want {\"location\": \"San Francisco\"}", resp.Output[0].Arguments)
	}
}

func TestTransformChatCompletionToResponse_MixedToolCallsAndText(t *testing.T) {
	chatResp := &ChatCompletionResponse{
		ID:      "chatcmpl-123",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "gpt-4",
		Choices: []ChatChoice{
			{
				Index: 0,
				Message: ChatMessage{
					Role:    "assistant",
					Content: "Here's the information you requested:",
					ToolCalls: []ChatToolCall{
						{
							ID:   "call_abc123",
							Type: "function",
							Function: ChatFunctionCall{
								Name:      "get_weather",
								Arguments: `{"location": "San Francisco"}`,
							},
						},
					},
				},
				FinishReason: "stop",
			},
		},
	}

	resp := TransformChatCompletionToResponse(chatResp, "resp_test123", "gpt-4")

	// Should have both function call and message outputs
	if len(resp.Output) != 2 {
		t.Fatalf("got %d output items, want 2", len(resp.Output))
	}

	// Check for function call item
	var funcCallItem *OutputItem
	var messageItem *OutputItem

	for i := range resp.Output {
		switch resp.Output[i].Type {
		case ItemTypeFunctionCall:
			funcCallItem = &resp.Output[i]
		case ItemTypeMessage:
			messageItem = &resp.Output[i]
		}
	}

	if funcCallItem == nil {
		t.Fatal("expected function call item in output")
	}
	if messageItem == nil {
		t.Fatal("expected message item in output")
	}

	// Verify function call details
	if funcCallItem.CallID != "call_abc123" {
		t.Errorf("function call ID = %s, want call_abc123", funcCallItem.CallID)
	}
	if funcCallItem.Name != "get_weather" {
		t.Errorf("function name = %s, want get_weather", funcCallItem.Name)
	}
	if funcCallItem.Arguments != `{"location": "San Francisco"}` {
		t.Errorf("function arguments = %s, want {\"location\": \"San Francisco\"}", funcCallItem.Arguments)
	}

	// Verify message details
	if messageItem.Role != "assistant" {
		t.Errorf("message role = %s, want assistant", messageItem.Role)
	}

	// Check if the message contains the expected text
	foundText := false
	for _, contentPart := range messageItem.Content {
		if contentPart.Type == ContentTypeOutputText && contentPart.Text == "Here's the information you requested:" {
			foundText = true
			break
		}
	}
	if !foundText {
		t.Errorf("expected message content 'Here's the information you requested:', but not found")
	}

	// Verify OutputText field
	if resp.OutputText != "Here's the information you requested:" {
		t.Errorf("OutputText = %s, want 'Here's the information you requested:'", resp.OutputText)
	}
}

func TestParseInput_InvalidJSON(t *testing.T) {
	_, err := parseInput(json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestGenerateIDs(t *testing.T) {
	// Test that IDs have correct prefixes
	respID := GenerateResponseID()
	if !startsWith(respID, "resp_") {
		t.Errorf("response ID should start with resp_, got %s", respID)
	}

	itemID := GenerateItemID()
	if !startsWith(itemID, "item_") {
		t.Errorf("item ID should start with item_, got %s", itemID)
	}

	msgID := GenerateMessageID()
	if !startsWith(msgID, "msg_") {
		t.Errorf("message ID should start with msg_, got %s", msgID)
	}

	callID := GenerateCallID()
	if !startsWith(callID, "call_") {
		t.Errorf("call ID should start with call_, got %s", callID)
	}

	// Test uniqueness
	id1 := GenerateResponseID()
	id2 := GenerateResponseID()
	if id1 == id2 {
		t.Error("generated IDs should be unique")
	}
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
