package anthropic

import (
	"fmt"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/inference/scheduling"
	"github.com/docker/model-runner/pkg/internal/utils"
	"github.com/docker/model-runner/pkg/logging"
	"github.com/docker/model-runner/pkg/middleware"
)

const (
	// APIPrefix is the prefix for Anthropic API routes.
	// llama.cpp implements Anthropic API at /v1/messages, matching the official Anthropic API structure.
	APIPrefix = "/anthropic"

	// maxRequestBodySize is the maximum allowed size for request bodies (10MB).
	maxRequestBodySize = 10 * 1024 * 1024
)

// Handler implements the Anthropic Messages API compatibility layer.
// It forwards requests to the scheduler which proxies to llama.cpp,
// which natively supports the Anthropic Messages API format.
type Handler struct {
	log           logging.Logger
	router        *http.ServeMux
	httpHandler   http.Handler
	modelManager  *models.Manager
	schedulerHTTP *scheduling.HTTPHandler
}

// NewHandler creates a new Anthropic API handler.
func NewHandler(log logging.Logger, schedulerHTTP *scheduling.HTTPHandler, allowedOrigins []string, modelManager *models.Manager) *Handler {
	h := &Handler{
		log:           log,
		router:        http.NewServeMux(),
		schedulerHTTP: schedulerHTTP,
		modelManager:  modelManager,
	}

	// Register routes
	for route, handler := range h.routeHandlers() {
		h.router.HandleFunc(route, handler)
	}

	// Apply CORS middleware
	h.httpHandler = middleware.CorsMiddleware(allowedOrigins, h.router)

	return h
}

// ServeHTTP implements the http.Handler interface.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	safeMethod := utils.SanitizeForLog(r.Method, -1)
	safePath := utils.SanitizeForLog(r.URL.Path, -1)
	h.log.Info(fmt.Sprintf("Anthropic API request: %s %s", safeMethod, safePath))
	h.httpHandler.ServeHTTP(w, r)
}

// routeHandlers returns the mapping of routes to their handlers.
func (h *Handler) routeHandlers() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		// Messages API endpoint - main chat completion endpoint
		"POST " + APIPrefix + "/v1/messages": h.handleMessages,
		// Token counting endpoint
		"POST " + APIPrefix + "/v1/messages/count_tokens": h.handleCountTokens,
	}
}

// MessagesRequest represents an Anthropic Messages API request.
// This is used to extract the model field for routing purposes.
type MessagesRequest struct {
	Model string `json:"model"`
}

// handleMessages handles POST /anthropic/v1/messages requests.
// It forwards the request to the scheduler which proxies to the llama.cpp backend.
// The llama.cpp backend natively handles the Anthropic Messages API format conversion.
func (h *Handler) handleMessages(w http.ResponseWriter, r *http.Request) {
	h.proxyToBackend(w, r, "/v1/messages")
}

// handleCountTokens handles POST /anthropic/v1/messages/count_tokens requests.
// It forwards the request to the scheduler which proxies to the llama.cpp backend.
func (h *Handler) handleCountTokens(w http.ResponseWriter, r *http.Request) {
	h.proxyToBackend(w, r, "/v1/messages/count_tokens")
}

// proxyToBackend proxies the request to the llama.cpp backend via the scheduler.
func (h *Handler) proxyToBackend(w http.ResponseWriter, r *http.Request, targetPath string) {
	ctx := r.Context()

	// Read the request body
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBodySize))
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			h.writeAnthropicError(w, http.StatusRequestEntityTooLarge, "request_too_large", "Request body too large")
		} else {
			h.writeAnthropicError(w, http.StatusInternalServerError, "internal_error", "Failed to read request body")
		}
		return
	}

	// Parse the model field from the request to route to the correct backend
	var req MessagesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "Invalid JSON in request body")
		return
	}

	if req.Model == "" {
		h.writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "Missing required field: model")
		return
	}

	// Use model name from request
	modelName := req.Model

	// Verify the model exists locally
	_, err = h.modelManager.GetLocal(modelName)
	if err != nil {
		h.writeAnthropicError(w, http.StatusNotFound, "not_found_error", "Model not found: "+modelName)
		return
	}

	// Create the proxied request to the inference endpoint
	// The scheduler will route to the appropriate backend
	newReq := r.Clone(ctx)
	newReq.URL.Path = inference.InferencePrefix + targetPath
	newReq.Body = io.NopCloser(bytes.NewReader(body))
	newReq.ContentLength = int64(len(body))
	newReq.Header.Set("Content-Type", "application/json")
	newReq.Header.Set(inference.RequestOriginHeader, inference.OriginAnthropicMessages)

	// Forward to the scheduler HTTP handler
	h.schedulerHTTP.ServeHTTP(w, newReq)
}

// AnthropicError represents an error response in the Anthropic API format.
type AnthropicError struct {
	Type  string            `json:"type"`
	Error AnthropicErrorObj `json:"error"`
}

// AnthropicErrorObj represents the error object in an Anthropic error response.
type AnthropicErrorObj struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// writeAnthropicError writes an error response in the Anthropic API format.
func (h *Handler) writeAnthropicError(w http.ResponseWriter, statusCode int, errorType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errResp := AnthropicError{
		Type: "error",
		Error: AnthropicErrorObj{
			Type:    errorType,
			Message: message,
		},
	}

	if err := json.NewEncoder(w).Encode(errResp); err != nil {
		h.log.Error(fmt.Sprintf("Failed to encode error response: %v", err))
	}
}
