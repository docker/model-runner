package anthropic

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestWriteAnthropicError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		errorType  string
		message    string
		wantBody   string
	}{
		{
			name:       "invalid request error",
			statusCode: http.StatusBadRequest,
			errorType:  "invalid_request_error",
			message:    "Missing required field: model",
			wantBody:   `{"type":"error","error":{"type":"invalid_request_error","message":"Missing required field: model"}}`,
		},
		{
			name:       "not found error",
			statusCode: http.StatusNotFound,
			errorType:  "not_found_error",
			message:    "Model not found: test-model",
			wantBody:   `{"type":"error","error":{"type":"not_found_error","message":"Model not found: test-model"}}`,
		},
		{
			name:       "internal error",
			statusCode: http.StatusInternalServerError,
			errorType:  "internal_error",
			message:    "An internal error occurred",
			wantBody:   `{"type":"error","error":{"type":"internal_error","message":"An internal error occurred"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			discard := logrus.New()
			discard.SetOutput(io.Discard)
			h := &Handler{log: logrus.NewEntry(discard)}
			h.writeAnthropicError(rec, tt.statusCode, tt.errorType, tt.message)

			if rec.Code != tt.statusCode {
				t.Errorf("expected status %d, got %d", tt.statusCode, rec.Code)
			}

			if contentType := rec.Header().Get("Content-Type"); contentType != "application/json" {
				t.Errorf("expected Content-Type application/json, got %s", contentType)
			}

			body := strings.TrimSpace(rec.Body.String())
			if body != tt.wantBody {
				t.Errorf("expected body %s, got %s", tt.wantBody, body)
			}
		})
	}
}

func TestRouteHandlers(t *testing.T) {
	t.Parallel()

	h := &Handler{
		router: http.NewServeMux(),
	}

	routes := h.routeHandlers()

	expectedRoutes := []string{
		"POST " + APIPrefix + "/v1/messages",
		"POST " + APIPrefix + "/v1/messages/count_tokens",
	}

	for _, route := range expectedRoutes {
		if _, exists := routes[route]; !exists {
			t.Errorf("expected route %s to be registered", route)
		}
	}

	if len(routes) != len(expectedRoutes) {
		t.Errorf("expected %d routes, got %d", len(expectedRoutes), len(routes))
	}
}

func TestAPIPrefix(t *testing.T) {
	t.Parallel()

	if APIPrefix != "/anthropic" {
		t.Errorf("expected APIPrefix to be /anthropic, got %s", APIPrefix)
	}
}

func TestProxyToBackend_InvalidJSON(t *testing.T) {
	t.Parallel()

	discard := logrus.New()
	discard.SetOutput(io.Discard)
	h := &Handler{log: logrus.NewEntry(discard)}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", strings.NewReader(`{invalid json`))

	h.proxyToBackend(rec, req, "/v1/messages")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "invalid_request_error") {
		t.Errorf("expected body to contain 'invalid_request_error', got %s", body)
	}
	if !strings.Contains(body, "Invalid JSON") {
		t.Errorf("expected body to contain 'Invalid JSON', got %s", body)
	}
}

func TestProxyToBackend_MissingModel(t *testing.T) {
	t.Parallel()

	discard := logrus.New()
	discard.SetOutput(io.Discard)
	h := &Handler{log: logrus.NewEntry(discard)}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", strings.NewReader(`{"messages": []}`))

	h.proxyToBackend(rec, req, "/v1/messages")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "invalid_request_error") {
		t.Errorf("expected body to contain 'invalid_request_error', got %s", body)
	}
	if !strings.Contains(body, "Missing required field: model") {
		t.Errorf("expected body to contain 'Missing required field: model', got %s", body)
	}
}

func TestProxyToBackend_EmptyModel(t *testing.T) {
	t.Parallel()

	discard := logrus.New()
	discard.SetOutput(io.Discard)
	h := &Handler{log: logrus.NewEntry(discard)}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", strings.NewReader(`{"model": ""}`))

	h.proxyToBackend(rec, req, "/v1/messages")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "invalid_request_error") {
		t.Errorf("expected body to contain 'invalid_request_error', got %s", body)
	}
	if !strings.Contains(body, "Missing required field: model") {
		t.Errorf("expected body to contain 'Missing required field: model', got %s", body)
	}
}

func TestProxyToBackend_RequestTooLarge(t *testing.T) {
	t.Parallel()

	discard := logrus.New()
	discard.SetOutput(io.Discard)
	h := &Handler{log: logrus.NewEntry(discard)}

	// Create a request body that exceeds the maxRequestBodySize (10MB)
	// We'll use a reader that simulates a large body without actually allocating it
	largeBody := strings.NewReader(`{"model": "test-model", "data": "` + strings.Repeat("x", maxRequestBodySize+1) + `"}`)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", largeBody)

	h.proxyToBackend(rec, req, "/v1/messages")

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status %d, got %d", http.StatusRequestEntityTooLarge, rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "request_too_large") {
		t.Errorf("expected body to contain 'request_too_large', got %s", body)
	}
}
