package routing

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestNewRouter_WithResponsesAPI_Close verifies that creating a router with
// IncludeResponsesAPI enabled and then calling Close does not leak
// goroutines. The goleak detector in TestMain will catch any leak.
func TestNewRouter_WithResponsesAPI_Close(t *testing.T) {
	log := slog.New(slog.DiscardHandler)

	result := NewRouter(RouterConfig{
		Log:                 log,
		IncludeResponsesAPI: true,
		// Scheduler, ModelHandler, etc. are nil — the responses handler
		// only needs them when actually serving requests, not for route
		// registration and cleanup.
	})

	// Verify the mux was created.
	if result.Mux == nil {
		t.Fatal("expected non-nil Mux")
	}

	// Close must stop the responses Store cleanup goroutine.
	result.Close()
}

// TestNewRouter_WithoutResponsesAPI_Close verifies that Close is safe
// to call even when the Responses API is not enabled (no closers).
func TestNewRouter_WithoutResponsesAPI_Close(t *testing.T) {
	log := slog.New(slog.DiscardHandler)

	result := NewRouter(RouterConfig{
		Log:                 log,
		IncludeResponsesAPI: false,
	})

	if result.Mux == nil {
		t.Fatal("expected non-nil Mux")
	}

	// Should be a no-op, must not panic.
	result.Close()
}

func TestNewRouter_ResponsesAPIRoutes(t *testing.T) {
	tests := []string{
		"/responses",
		"/v1/responses",
		"/engines/responses",
		"/engines/v1/responses",
		"/engines/llama.cpp/v1/responses",
	}

	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			log := slog.New(slog.DiscardHandler)

			result := NewRouter(RouterConfig{
				Log:                 log,
				IncludeResponsesAPI: true,
			})
			t.Cleanup(result.Close)

			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{
				"input": "Hello"
			}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			result.Mux.ServeHTTP(w, req)

			resp := w.Result()
			if resp.StatusCode != http.StatusBadRequest {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("status = %d, want %d, body: %s", resp.StatusCode, http.StatusBadRequest, body)
			}

			var errResp struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
				t.Fatalf("failed to decode error response: %v", err)
			}
			if errResp.Error.Code != "invalid_request" {
				t.Errorf("error.code = %s, want invalid_request", errResp.Error.Code)
			}
			if errResp.Error.Message != "model is required" {
				t.Errorf("error.message = %s, want model is required", errResp.Error.Message)
			}
		})
	}
}
