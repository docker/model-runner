package ollama

import (
	"encoding/json"
	"testing"
)

func TestConvertMessages_Multimodal(t *testing.T) {
	tests := []struct {
		name     string
		messages []Message
		expected string
	}{
		{
			name: "text only message",
			messages: []Message{
				{
					Role:    "user",
					Content: "Hello, world!",
				},
			},
			expected: `[{"content":"Hello, world!","role":"user"}]`,
		},
		{
			name: "multimodal message with text and image",
			messages: []Message{
				{
					Role:    "user",
					Content: "is there a person in the image? Answer yes or no",
					Images:  []string{"data:image/jpeg;base64,/9j/4AAQSkZJRgABAQEBLA...."},
				},
			},
			expected: `[{"content":[{"text":"is there a person in the image? Answer yes or no","type":"text"},{"image_url":{"url":"data:image/jpeg;base64,/9j/4AAQSkZJRgABAQEBLA...."},"type":"image_url"}],"role":"user"}]`,
		},
		{
			name: "multimodal message with only image (no text)",
			messages: []Message{
				{
					Role:    "user",
					Content: "",
					Images:  []string{"data:image/jpeg;base64,/9j/4AAQSkZJRgABAQEBLA...."},
				},
			},
			expected: `[{"content":[{"image_url":{"url":"data:image/jpeg;base64,/9j/4AAQSkZJRgABAQEBLA...."},"type":"image_url"}],"role":"user"}]`,
		},
		{
			name: "multimodal message with multiple images",
			messages: []Message{
				{
					Role:    "user",
					Content: "Compare these images",
					Images: []string{
						"data:image/jpeg;base64,image1...",
						"data:image/jpeg;base64,image2...",
					},
				},
			},
			expected: `[{"content":[{"text":"Compare these images","type":"text"},{"image_url":{"url":"data:image/jpeg;base64,image1..."},"type":"image_url"},{"image_url":{"url":"data:image/jpeg;base64,image2..."},"type":"image_url"}],"role":"user"}]`,
		},
		{
			name: "multimodal message with raw base64 from OpenWebUI (no prefix)",
			messages: []Message{
				{
					Role:    "user",
					Content: "is there a person in the image? Answer yes or no",
					Images:  []string{"/9j/4AAQSkZJRgABAQEBLA...."},
				},
			},
			// Should auto-add the data URI prefix
			expected: `[{"content":[{"text":"is there a person in the image? Answer yes or no","type":"text"},{"image_url":{"url":"data:image/jpeg;base64,/9j/4AAQSkZJRgABAQEBLA...."},"type":"image_url"}],"role":"user"}]`,
		},
		{
			name: "assistant message with tool calls",
			messages: []Message{
				{
					Role:    "assistant",
					Content: "Let me call a function",
					ToolCalls: []ToolCall{
						{
							ID:   "call_123",
							Type: "function",
							Function: FunctionCall{
								Name:      "get_weather",
								Arguments: map[string]interface{}{"location": "San Francisco"},
							},
						},
					},
				},
			},
			// The tool_calls will have arguments converted to JSON string
			// Note: JSON field order follows struct definition
			expected: `[{"content":"Let me call a function","role":"assistant","tool_calls":[{"id":"call_123","type":"function","function":{"name":"get_weather","arguments":"{\"location\":\"San Francisco\"}"}}]}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertMessages(tt.messages)

			// Marshal to JSON for comparison
			resultJSON, err := json.Marshal(result)
			if err != nil {
				t.Fatalf("Failed to marshal result: %v", err)
			}

			// Compare JSON strings
			if string(resultJSON) != tt.expected {
				t.Errorf("convertMessages() mismatch\nGot:      %s\nExpected: %s", string(resultJSON), tt.expected)
			}
		})
	}
}

func TestEnsureDataURIPrefix(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "raw base64 without prefix",
			input:    "/9j/4AAQSkZJRgABAQEBLA...",
			expected: "data:image/jpeg;base64,/9j/4AAQSkZJRgABAQEBLA...",
		},
		{
			name:     "already has data URI prefix",
			input:    "data:image/jpeg;base64,/9j/4AAQSkZJRgABAQEBLA...",
			expected: "data:image/jpeg;base64,/9j/4AAQSkZJRgABAQEBLA...",
		},
		{
			name:     "already has data URI with png",
			input:    "data:image/png;base64,iVBORw0KGgo...",
			expected: "data:image/png;base64,iVBORw0KGgo...",
		},
		{
			name:     "http URL",
			input:    "http://example.com/image.jpg",
			expected: "http://example.com/image.jpg",
		},
		{
			name:     "https URL",
			input:    "https://example.com/image.jpg",
			expected: "https://example.com/image.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ensureDataURIPrefix(tt.input)
			if result != tt.expected {
				t.Errorf("ensureDataURIPrefix() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestConvertMessages_PreservesOrder(t *testing.T) {
	messages := []Message{
		{Role: "system", Content: "You are a helpful assistant"},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
		{Role: "user", Content: "What's in this image?", Images: []string{"data:image/jpeg;base64,abc123"}},
	}

	result := convertMessages(messages)

	if len(result) != 4 {
		t.Errorf("Expected 4 messages, got %d", len(result))
	}

	// Check roles are preserved in order
	expectedRoles := []string{"system", "user", "assistant", "user"}
	for i, msg := range result {
		if msg["role"] != expectedRoles[i] {
			t.Errorf("Message %d: expected role %s, got %s", i, expectedRoles[i], msg["role"])
		}
	}

	// Check last message has multimodal content
	lastMsg := result[3]
	content, ok := lastMsg["content"].([]map[string]interface{})
	if !ok {
		t.Errorf("Last message content should be an array, got %T", lastMsg["content"])
	}
	if len(content) != 2 {
		t.Errorf("Last message should have 2 content parts (text + image), got %d", len(content))
	}
}
