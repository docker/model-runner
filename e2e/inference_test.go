//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/docker/model-runner/pkg/inference/backends/llamacpp"
	"github.com/docker/model-runner/pkg/inference/backends/vllm"
	"github.com/docker/model-runner/pkg/inference/platform"
)

type backendTestCase struct {
	name  string
	model string
}

var backends = func() []backendTestCase {
	b := []backendTestCase{
		{llamacpp.Name, ggufModel},
	}
	if platform.SupportsVLLMMetal() {
		b = append(b, backendTestCase{vllm.Name, mlxModel})
	}
	return b
}()

func TestE2E_Inference(t *testing.T) {
	for _, bc := range backends {
		bc := bc
		t.Run(bc.name, func(t *testing.T) {
			t.Run("Pull", func(t *testing.T) {
				pullModel(t, bc.model)
				t.Logf("pulled %s", bc.model)
			})

			t.Run("ListModels", func(t *testing.T) {
				status, body := doJSON(t, http.MethodGet, serverURL+"/engines/v1/models", nil)
				if status != http.StatusOK {
					t.Fatalf("list: status=%d body=%s", status, body)
				}
				var result struct {
					Data []struct {
						ID string `json:"id"`
					} `json:"data"`
				}
				if err := json.Unmarshal(body, &result); err != nil {
					t.Fatalf("decode: %v", err)
				}
				found := false
				for _, m := range result.Data {
					if strings.Contains(m.ID, "smollm2") {
						found = true
					}
				}
				if !found {
					t.Fatalf("model %s not in list", bc.model)
				}
			})

			t.Run("ChatCompletion", func(t *testing.T) {
				resp := chatCompletion(t, bc.model, "Say hello in one word.")
				if len(resp.Choices) == 0 {
					t.Fatal("no choices")
				}
				t.Logf("response: %q", resp.Choices[0].Message.Content)
				if resp.Choices[0].Message.Content == "" {
					t.Fatal("empty content")
				}
				if resp.Choices[0].Message.Role != "assistant" {
					t.Errorf("expected role=assistant, got %q", resp.Choices[0].Message.Role)
				}
			})

			t.Run("ChatCompletionStreaming", func(t *testing.T) {
				content := streamingChatCompletion(t, bc.model, "Count from 1 to 3.")
				t.Logf("streamed: %q", content)
			})

			t.Run("ResponsesAPI", func(t *testing.T) {
				status, body := doJSON(t, http.MethodPost, serverURL+"/responses",
					map[string]any{
						"model": bc.model,
						"input": "Say hi in one word.",
					})
				if status != http.StatusOK {
					t.Fatalf("responses API: status=%d body=%s", status, body)
				}
				var resp struct {
					ID         string `json:"id"`
					Status     string `json:"status"`
					OutputText string `json:"output_text"`
				}
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("decode: %v (body=%s)", err, body)
				}
				if resp.Status != "completed" {
					t.Errorf("expected status=completed, got %q", resp.Status)
				}
				if resp.OutputText == "" {
					t.Fatal("empty output_text")
				}
				t.Logf("responses API: %q", resp.OutputText)
			})

			t.Run("AnthropicMessages", func(t *testing.T) {
				status, body := doJSON(t, http.MethodPost, serverURL+"/anthropic/v1/messages",
					map[string]any{
						"model":      bc.model,
						"max_tokens": 32,
						"messages":   []map[string]string{{"role": "user", "content": "Say hi."}},
					})
				if status != http.StatusOK {
					t.Fatalf("anthropic messages: status=%d body=%s", status, body)
				}
				var resp struct {
					Content []struct {
						Text string `json:"text"`
					} `json:"content"`
				}
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("decode: %v (body=%s)", err, body)
				}
				if len(resp.Content) == 0 || resp.Content[0].Text == "" {
					t.Fatal("empty anthropic response")
				}
				t.Logf("anthropic: %q", resp.Content[0].Text)
			})

			t.Run("OllamaChat", func(t *testing.T) {
				resp := ollamaChat(t, bc.model, "Say hi.")
				if resp.Message.Content == "" {
					t.Fatal("empty ollama chat response")
				}
				if !resp.Done {
					t.Error("ollama chat response not done")
				}
				t.Logf("ollama chat: %q", resp.Message.Content)
			})

			t.Run("OllamaGenerate", func(t *testing.T) {
				resp := ollamaGenerate(t, bc.model, "Say hi.")
				if resp.Response == "" {
					t.Fatal("empty ollama generate response")
				}
				if !resp.Done {
					t.Error("ollama generate response not done")
				}
				t.Logf("ollama generate: %q", resp.Response)
			})

			t.Run("OllamaTags", func(t *testing.T) {
				status, body := doJSON(t, http.MethodGet, serverURL+"/api/tags", nil)
				if status != http.StatusOK {
					t.Fatalf("tags: status=%d body=%s", status, body)
				}
				var list struct {
					Models []struct {
						Name string `json:"name"`
					} `json:"models"`
				}
				if err := json.Unmarshal(body, &list); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if len(list.Models) == 0 {
					t.Fatal("no models in /api/tags")
				}
				t.Logf("tags returned %d models", len(list.Models))
			})

			t.Run("Remove", func(t *testing.T) {
				out, err := runCLI(t, "unload", "--all")
				if err != nil {
					t.Fatalf("cli unload failed: %v\noutput: %s", err, out)
				}
				removeModel(t, bc.model)
				t.Logf("removed %s", bc.model)
			})
		})
	}
}
