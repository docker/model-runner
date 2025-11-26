package metrics

import (
	"encoding/json"
	"testing"

	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/sirupsen/logrus"
)

func TestTruncateMediaFields(t *testing.T) {
	// Create a mock logger and model manager
	logger := logrus.New()
	modelManager := &models.Manager{}
	recorder := NewOpenAIRecorder(logger, modelManager)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Invalid JSON scenarios
		{
			name:     "Completely malformed JSON",
			input:    `{invalid json: "missing quotes and brackets`,
			expected: `{invalid json: "missing quotes and brackets`,
		},
		{
			name:     "Truncated JSON",
			input:    `{"model": "test", "messages": [{"role": "user", "content": `,
			expected: `{"model": "test", "messages": [{"role": "user", "content": `,
		},
		{
			name:     "Valid JSON but wrong structure - no messages field",
			input:    `{"model": "test-model", "prompt": "This uses wrong field name"}`,
			expected: `{"model": "test-model", "prompt": "This uses wrong field name"}`,
		},
		{
			name:     "Messages field is not an array",
			input:    `{"model": "test-model", "messages": {"role": "user", "content": "text"}}`,
			expected: `{"model": "test-model", "messages": {"role": "user", "content": "text"}}`,
		},
		{
			name:     "Content is string instead of array (no media to truncate)",
			input:    `{"model": "test-model", "messages": [{"role": "user", "content": "simple string"}]}`,
			expected: `{"messages":[{"content":"simple string","role":"user"}],"model":"test-model"}`,
		},
		{
			name:     "Empty messages array",
			input:    `{"model": "test-model", "messages": []}`,
			expected: `{"messages":[],"model":"test-model"}`,
		},
		{
			name:     "Null messages field",
			input:    `{"model": "test-model", "messages": null}`,
			expected: `{"model": "test-model", "messages": null}`,
		},
		{
			name:     "Message with null content",
			input:    `{"model": "test-model", "messages": [{"role": "user", "content": null}]}`,
			expected: `{"messages":[{"content":null,"role":"user"}],"model":"test-model"}`,
		},
		{
			name:     "Mixed content types in array",
			input:    `{"model": "test-model", "messages": [{"role": "user", "content": [{"type": "text", "text": "hello"}, "invalid string in array"]}]}`,
			expected: `{"messages":[{"content":[{"text":"hello","type":"text"},"invalid string in array"],"role":"user"}],"model":"test-model"}`,
		},
		{
			name: "Image URL with base64 data",
			input: `{
				"model": "test-model",
				"messages": [
					{
						"role": "user",
						"content": [
							{
								"type": "text",
								"text": "describe the image"
							},
							{
								"type": "image_url",
								"image_url": {
									"url": "data:image/jpeg;base64,/9j/4AAQSkZJRgABAQAAAQABAAD/2wCEAAkGBxASERATExIWFhMXGRwVGBgYFRgTExIQFRIXGBUVGxYZICggGh0lHRYVITEiJTArLi4uGB8zODMtNygtLisBCgoKDg0OFQ8QGy0dHR0tNy0tKy0rLS0tLSstLSstKy0rLS0tLSsrLS0rLS03LS0tKy03LS0tLSstLS0rLTctK"
								}
							}
						]
					}
				]
			}`,
			expected: "{\"messages\":[{\"content\":[{\"text\":\"describe the image\",\"type\":\"text\"},{\"image_url\":{\"url\":\"data:image/jpeg;base64,/9j/4AAQSkZJRgABAQAAAQABAAD/2wCEAAkGBxASERATExIWFhMXGRwVGBgYFRgTExIQFRIXGBUVGxYZICggGh0lHRYVITEiJTAr...[truncated 105 chars]\"},\"type\":\"image_url\"}],\"role\":\"user\"}],\"model\":\"test-model\"}",
		},
		{
			name: "Audio data",
			input: `{
				"model": "test-model",
				"messages": [
					{
						"role": "user",
						"content": [
							{
								"type": "text",
								"text": "transcribe this audio"
							},
							{
								"type": "input_audio",
								"input_audio": {
									"data": "UklGRoqxAgBXQVZFZm10IBAAAAABAAEAQB8AAIA+AAACABAATElTVBoAAABJTkZPSVNGVA4AAABMYXZmNTguNzYuMTAwAGRhdGFEsQIAsf+Y/2f/Uf83/y//Gf8g/xf/I/8r/0//Vv99/4z/r//H//r/DAAkACwAPAA7AE8ATQBIADMAKgAQAAUA6P/m/93//P8pAE4AXQBuAF8AbQByAKYAwQC/AKcArgCOAJMAoAClAIMAeQBKABgA/f/7/9z/wv+h/33/S/9S/0r/Uv9P/2L/S/9a/"
								}
							}
						]
					}
				]
			}`,
			expected: "{\"messages\":[{\"content\":[{\"text\":\"transcribe this audio\",\"type\":\"text\"},{\"input_audio\":{\"data\":\"UklGRoqxAgBXQVZFZm10IBAAAAABAAEAQB8AAIA+AAACABAATElTVBoAAABJTkZPSVNGVA4AAABMYXZmNTguNzYuMTAwAGRhdGFE...[truncated 185 chars]\"},\"type\":\"input_audio\"}],\"role\":\"user\"}],\"model\":\"test-model\"}",
		},
		{
			name: "Regular request without media",
			input: `{
				"model": "test-model",
				"messages": [
					{
						"role": "user",
						"content": "Hello, how are you?"
					}
				]
			}`,
			expected: "{\"messages\":[{\"content\":\"Hello, how are you?\",\"role\":\"user\"}],\"model\":\"test-model\"}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := recorder.truncateMediaFields([]byte(tt.input))
			resultStr := string(result)

			// Check if input is valid JSON
			var inputJSON interface{}
			inputErr := json.Unmarshal([]byte(tt.input), &inputJSON)

			if inputErr != nil {
				// For invalid JSON inputs, verify it's returned unchanged
				if resultStr != tt.expected {
					t.Errorf("Invalid JSON should be returned unchanged. Expected %q, got %q", tt.expected, resultStr)
				}
			} else {
				// For valid JSON inputs, verify output is still valid JSON
				var resultJSON interface{}
				if err := json.Unmarshal(result, &resultJSON); err != nil {
					t.Errorf("Result should be valid JSON, but got error: %v", err)
				}

				// Also check the content matches expected
				if resultStr != tt.expected {
					t.Errorf("Expected result %q, but got %q", tt.expected, resultStr)
				}
			}
		})
	}
}

func TestTruncateBase64Data(t *testing.T) {
	logger := logrus.New()
	modelManager := &models.Manager{}
	recorder := NewOpenAIRecorder(logger, modelManager)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Short data URL",
			input:    "data:image/jpeg;base64,abc123",
			expected: "data:image/jpeg;base64,abc123",
		},
		{
			name:     "Long data URL",
			input:    "data:image/jpeg;base64," + generateLongString(200),
			expected: "data:image/jpeg;base64," + generateLongString(100) + "...[truncated 100 chars]",
		},
		{
			name:     "Long prefix data URL with short base64 data",
			input:    "data:image/png;charset=utf-8;metadata=very-long-metadata-string-that-makes-the-prefix-longer;base64," + generateLongString(50),
			expected: "data:image/png;charset=utf-8;metadata=very-long-metadata-string-that-makes-the-prefix-longer;base64," + generateLongString(50),
		},
		{
			name:     "Long raw base64",
			input:    generateLongString(200),
			expected: generateLongString(100) + "...[truncated 100 chars]",
		},
		{
			name:     "Short raw base64",
			input:    "abc123",
			expected: "abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := recorder.truncateBase64Data(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// Helper function to generate a string of specified length
func generateLongString(length int) string {
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		result[i] = 'A' + byte(i%26)
	}
	return string(result)
}
