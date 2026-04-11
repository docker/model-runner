//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/docker/model-runner/cmd/cli/desktop"
)

func TestE2E_APIErrors(t *testing.T) {
	t.Run("InvalidModel", func(t *testing.T) {
		status, body := doJSON(t, http.MethodPost, serverURL+"/engines/v1/chat/completions",
			desktop.OpenAIChatRequest{
				Model:    "nonexistent/model:latest",
				Messages: []desktop.OpenAIChatMessage{{Role: "user", Content: "hi"}},
			})
		if status == http.StatusOK {
			t.Fatalf("expected error for invalid model, got 200: %s", body)
		}
		t.Logf("invalid model: status=%d", status)
	})

	t.Run("MalformedJSON", func(t *testing.T) {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost,
			serverURL+"/engines/v1/chat/completions",
			strings.NewReader(`{bad json`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Fatal("expected error for malformed JSON")
		}
		t.Logf("malformed JSON: status=%d", resp.StatusCode)
	})

	t.Run("EmptyMessages", func(t *testing.T) {
		status, body := doJSON(t, http.MethodPost, serverURL+"/engines/v1/chat/completions",
			desktop.OpenAIChatRequest{
				Model:    ggufModel,
				Messages: []desktop.OpenAIChatMessage{},
			})
		// Some backends accept empty messages, some don't — just verify no 500.
		if status == http.StatusInternalServerError {
			t.Fatalf("unexpected 500 for empty messages: %s", body)
		}
		t.Logf("empty messages: status=%d", status)
	})
}

func TestE2E_ModelLifecycle(t *testing.T) {
	model := ggufModel

	t.Run("PullIdempotent", func(t *testing.T) {
		pullModel(t, model)
		output := pullModel(t, model)
		if !strings.Contains(output, "Using cached model") {
			t.Errorf("expected 'Using cached model' in second pull, got: %s", output)
		}
	})

	t.Run("InspectModel", func(t *testing.T) {
		status, body := doJSON(t, http.MethodGet,
			serverURL+"/engines/v1/models/"+model, nil)
		if status != http.StatusOK {
			t.Fatalf("inspect: status=%d body=%s", status, body)
		}
		var m struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(body, &m); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if m.ID == "" {
			t.Fatal("empty model ID in inspect response")
		}
		t.Logf("inspect: id=%s", m.ID)
	})

	t.Run("OllamaShow", func(t *testing.T) {
		status, body := doJSON(t, http.MethodPost, serverURL+"/api/show",
			map[string]string{"name": model})
		if status != http.StatusOK {
			t.Fatalf("show: status=%d body=%s", status, body)
		}
		var show struct {
			Details struct {
				Format string `json:"format"`
			} `json:"details"`
		}
		if err := json.Unmarshal(body, &show); err != nil {
			t.Fatalf("decode: %v", err)
		}
		t.Logf("format: %s", show.Details.Format)
	})

	t.Run("Remove", func(t *testing.T) {
		removeModel(t, model)
	})
}
