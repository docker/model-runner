package commands

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/docker/model-runner/cmd/cli/desktop" // Add this import
	"github.com/spf13/cobra"
)

func TestChatWithNIM_Context(t *testing.T) {
	// Save original port and restore after test
	originalPort := nimDefaultPort
	defer func() { nimDefaultPort = originalPort }()

	// Track received messages
	var receivedPayloads []struct {
		Messages []desktop.OpenAIChatMessage `json:"messages"`
	}

	// Setup Mock Server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("Expected path /v1/chat/completions, got %s", r.URL.Path)
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Failed to read request body: %v", err)
		}

		var payload struct {
			Messages []desktop.OpenAIChatMessage `json:"messages"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("Failed to unmarshal request body: %v", err)
		}

		receivedPayloads = append(receivedPayloads, payload)

		// Mock response (SSE format)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(`data: {"choices":[{"delta":{"content":"Response"}}]}
`))
		w.Write([]byte(`data: [DONE]
`))
	}))
	defer server.Close()

	// Parse server URL to get the port
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("Failed to parse server URL: %v", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("Failed to parse port: %v", err)
	}
	nimDefaultPort = port

	// Initialize messages slice
	var messages []desktop.OpenAIChatMessage
	cmd := &cobra.Command{}

	// First interaction
	err = chatWithNIM(cmd, "ai/model", &messages, "Hello")
	if err != nil {
		t.Fatalf("First chatWithNIM failed: %v", err)
	}

	// Verify first request
	if len(receivedPayloads) != 1 {
		t.Fatalf("Expected 1 request, got %d", len(receivedPayloads))
	}
	if len(receivedPayloads[0].Messages) != 1 {
		t.Errorf("Expected 1 message in first request, got %d", len(receivedPayloads[0].Messages))
	}
	if receivedPayloads[0].Messages[0].Content != "Hello" {
		t.Errorf("Expected content 'Hello', got '%s'", receivedPayloads[0].Messages[0].Content)
	}

	// Second interaction
	err = chatWithNIM(cmd, "ai/model", &messages, "How are you?")
	if err != nil {
		t.Fatalf("Second chatWithNIM failed: %v", err)
	}

	// Verify second request
	if len(receivedPayloads) != 2 {
		t.Fatalf("Expected 2 requests, got %d", len(receivedPayloads))
	}
	
	// This is where we expect it to fail if the issue exists
	// We expect:
	// 1. User: Hello
	// 2. Assistant: Response
	// 3. User: How are you?
	if len(receivedPayloads[1].Messages) != 3 {
		t.Errorf("Expected 3 messages in second request, got %d", len(receivedPayloads[1].Messages))
		for i, m := range receivedPayloads[1].Messages {
			t.Logf("Message %d: Role=%s, Content=%s", i, m.Role, m.Content)
		}
	} else {
		// Verify message content
		if receivedPayloads[1].Messages[0].Content != "Hello" {
			t.Errorf("Msg 0: Expected 'Hello', got '%s'", receivedPayloads[1].Messages[0].Content)
		}
		if receivedPayloads[1].Messages[1].Role != "assistant" {
			t.Errorf("Msg 1: Expected role 'assistant', got '%s'", receivedPayloads[1].Messages[1].Role)
		}
		if receivedPayloads[1].Messages[2].Content != "How are you?" {
			t.Errorf("Msg 2: Expected 'How are you?', got '%s'", receivedPayloads[1].Messages[2].Content)
		}
	}
}
