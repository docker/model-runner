package ollama

import (
	"encoding/json"
	"fmt"
	"strings"
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
		{
			name: "tool result message with tool_call_id",
			messages: []Message{
				{
					Role:       "tool",
					Content:    "The weather in San Francisco is sunny, 72°F",
					ToolCallID: "call_123",
				},
			},
			expected: `[{"content":"The weather in San Francisco is sunny, 72°F","role":"tool","tool_call_id":"call_123"}]`,
		},
		{
			name: "multiple raw base64 images without prefix",
			messages: []Message{
				{
					Role:    "user",
					Content: "Compare these two images",
					Images: []string{
						"/9j/4AAQSkZJRgABAQEBLA...",
						"iVBORw0KGgoAAAANSUhEUgAAA...",
					},
				},
			},
			// Should auto-detect MIME types and add appropriate prefixes
			expected: `[{"content":[{"text":"Compare these two images","type":"text"},{"image_url":{"url":"data:image/jpeg;base64,/9j/4AAQSkZJRgABAQEBLA..."},"type":"image_url"},{"image_url":{"url":"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAA..."},"type":"image_url"}],"role":"user"}]`,
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
			name:     "raw JPEG base64 without prefix",
			input:    "/9j/4AAQSkZJRgABAQEBLA...",
			expected: "data:image/jpeg;base64,/9j/4AAQSkZJRgABAQEBLA...",
		},
		{
			name:     "raw PNG base64 without prefix",
			input:    "iVBORw0KGgoAAAANSUhEUgAAA...",
			expected: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAA...",
		},
		{
			name:     "raw GIF base64 without prefix",
			input:    "R0lGODlhAQABAIAAAAAAAP...",
			expected: "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP...",
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
		{
			name:     "empty string",
			input:    "",
			expected: "data:image/jpeg;base64,",
		},
		{
			name:     "whitespace with base64",
			input:    "  /9j/4AAQSkZJRgABAQEBLA...  ",
			expected: "data:image/jpeg;base64,/9j/4AAQSkZJRgABAQEBLA...",
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

func TestHandleVersion(t *testing.T) {
	// Verify version is >= 0.6.4 (minimum required by VSCode Copilot Chat)
	version := ollamaCompatVersion
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		t.Fatalf("Expected semver format, got %s", version)
	}

	major := 0
	minor := 0
	patch := 0
	fmt.Sscanf(parts[0], "%d", &major)
	fmt.Sscanf(parts[1], "%d", &minor)
	fmt.Sscanf(parts[2], "%d", &patch)

	// Must be >= 0.6.4
	versionNum := major*10000 + minor*100 + patch
	minimumNum := 0*10000 + 6*100 + 4
	if versionNum < minimumNum {
		t.Errorf("ollamaCompatVersion %s is below minimum 0.6.4 required by VSCode Copilot Chat", version)
	}
}

func TestShowResponseHasRequiredFields(t *testing.T) {
	// Verify ShowResponse struct can marshal the fields required by VSCode Copilot Chat
	response := ShowResponse{
		Details: ModelDetails{
			Format:            "gguf",
			Family:            "llama",
			Families:          []string{"llama"},
			ParameterSize:     "8B",
			QuantizationLevel: "Q4_K_M",
		},
		Template:     "{{ .System }}\n{{ .Prompt }}",
		Capabilities: []string{"completion", "tools"},
		ModelInfo: map[string]interface{}{
			"general.architecture":   "llama",
			"general.basename":       "ai/test-model:latest",
			"llama.context_length":   4096,
			"llama.embedding_length": 2048,
		},
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal ShowResponse: %v", err)
	}

	// Verify all expected fields are present in JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// Check capabilities field exists and is an array
	caps, ok := parsed["capabilities"]
	if !ok {
		t.Error("capabilities field missing from ShowResponse JSON")
	}
	capsArr, ok := caps.([]interface{})
	if !ok {
		t.Error("capabilities should be an array")
	}
	if len(capsArr) != 2 {
		t.Errorf("Expected 2 capabilities, got %d", len(capsArr))
	}

	// Check model_info field exists and has required keys
	modelInfo, ok := parsed["model_info"]
	if !ok {
		t.Error("model_info field missing from ShowResponse JSON")
	}
	infoMap, ok := modelInfo.(map[string]interface{})
	if !ok {
		t.Error("model_info should be a map")
	}
	if _, ok := infoMap["general.architecture"]; !ok {
		t.Error("model_info missing general.architecture")
	}
	if _, ok := infoMap["general.basename"]; !ok {
		t.Error("model_info missing general.basename")
	}

	// Check template field exists
	if _, ok := parsed["template"]; !ok {
		t.Error("template field missing from ShowResponse JSON")
	}

	// Check details.family field exists (required by VSCode Copilot)
	details, ok := parsed["details"].(map[string]interface{})
	if !ok {
		t.Error("details should be a map")
	}
	if _, ok := details["family"]; !ok {
		t.Error("details.family missing from ShowResponse JSON")
	}
}

func TestContainsString(t *testing.T) {
	tests := []struct {
		slice    []string
		s        string
		expected bool
	}{
		{[]string{"a", "b", "c"}, "b", true},
		{[]string{"a", "b", "c"}, "d", false},
		{[]string{}, "a", false},
		{[]string{"completion", "tools", "vision"}, "vision", true},
	}
	for _, tt := range tests {
		if got := containsString(tt.slice, tt.s); got != tt.expected {
			t.Errorf("containsString(%v, %q) = %v, want %v", tt.slice, tt.s, got, tt.expected)
		}
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
