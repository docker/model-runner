package responses

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/docker/model-runner/pkg/logging"
	"github.com/docker/model-runner/pkg/middleware"
)

// HTTPHandler handles Responses API HTTP requests.
type HTTPHandler struct {
	log                 logging.Logger
	router              *http.ServeMux
	httpHandler         http.Handler
	schedulerHTTP       http.Handler
	store               *Store
	maxRequestBodyBytes int64
}

// NewHTTPHandler creates a new Responses API handler.
func NewHTTPHandler(log logging.Logger, schedulerHTTP http.Handler, allowedOrigins []string) *HTTPHandler {
	h := &HTTPHandler{
		log:                 log,
		router:              http.NewServeMux(),
		schedulerHTTP:       schedulerHTTP,
		store:               NewStore(DefaultTTL),
		maxRequestBodyBytes: 10 * 1024 * 1024, // Default to 10MB
	}

	// Register routes
	h.router.HandleFunc("POST "+APIPrefix, h.handleCreate)
	h.router.HandleFunc("GET "+APIPrefix+"/{id}", h.handleGet)
	h.router.HandleFunc("GET "+APIPrefix+"/{id}/input_items", h.handleListInputItems)
	h.router.HandleFunc("DELETE "+APIPrefix+"/{id}", h.handleDelete)
	// Also register /v1/responses routes
	h.router.HandleFunc("POST /v1"+APIPrefix, h.handleCreate)
	h.router.HandleFunc("GET /v1"+APIPrefix+"/{id}", h.handleGet)
	h.router.HandleFunc("GET /v1"+APIPrefix+"/{id}/input_items", h.handleListInputItems)
	h.router.HandleFunc("DELETE /v1"+APIPrefix+"/{id}", h.handleDelete)

	// Apply CORS middleware
	h.httpHandler = middleware.CorsMiddleware(allowedOrigins, h.router)

	return h
}

// ServeHTTP implements http.Handler.
func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cleanPath := strings.ReplaceAll(r.URL.Path, "\n", "")
	cleanPath = strings.ReplaceAll(cleanPath, "\r", "")
	h.log.Info("Responses API request", "method", r.Method, "path", cleanPath)
	h.httpHandler.ServeHTTP(w, r)
}

// handleCreate handles POST /responses (or /v1/responses).
func (h *HTTPHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	// Read request body with a configurable limit
	reader := http.MaxBytesReader(w, r.Body, h.maxRequestBodyBytes)
	body, err := io.ReadAll(reader)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			h.sendError(
				w,
				http.StatusRequestEntityTooLarge,
				"request_too_large",
				fmt.Sprintf("Request body too large (max %d bytes)", h.maxRequestBodyBytes),
			)
			return
		}

		h.sendError(w, http.StatusBadRequest, "invalid_request", "Failed to read request body")
		return
	}

	// Parse request
	var req CreateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.sendError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON: "+err.Error())
		return
	}

	// Validate required fields
	if req.Model == "" {
		h.sendError(w, http.StatusBadRequest, "invalid_request", "model is required")
		return
	}

	// Create a new response
	respID := GenerateResponseID()
	resp := NewResponse(respID, req.Model)
	resp.Instructions = nilIfEmpty(req.Instructions)
	resp.Temperature = req.Temperature
	resp.TopP = req.TopP
	resp.MaxOutputTokens = req.MaxOutputTokens
	resp.Tools = req.Tools
	resp.ToolChoice = req.ToolChoice
	resp.ParallelToolCalls = req.ParallelToolCalls
	resp.Metadata = req.Metadata
	if req.PreviousResponseID != "" {
		resp.PreviousResponseID = &req.PreviousResponseID
	}
	if req.ReasoningEffort != "" {
		resp.ReasoningEffort = &req.ReasoningEffort
	}
	if req.User != "" {
		resp.User = &req.User
	}

	// Transform to chat completion request
	chatReq, err := TransformRequestToChatCompletion(&req, h.store)
	if err != nil {
		h.sendError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	// Marshal chat request
	chatBody, err := MarshalChatCompletionRequest(chatReq)
	if err != nil {
		h.sendError(w, http.StatusInternalServerError, "internal_error", "Failed to marshal request")
		return
	}

	// Create upstream request
	upstreamReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "/engines/v1/chat/completions", bytes.NewReader(chatBody))
	if err != nil {
		h.sendError(w, http.StatusInternalServerError, "internal_error", "Failed to create request")
		return
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	// Copy relevant headers
	if auth := r.Header.Get("Authorization"); auth != "" {
		upstreamReq.Header.Set("Authorization", auth)
	}

	if req.Stream {
		// Handle streaming response
		h.handleStreaming(w, upstreamReq, resp)
	} else {
		// Handle non-streaming response
		h.handleNonStreaming(w, upstreamReq, resp)
	}
}

// handleStreaming handles streaming responses.
func (h *HTTPHandler) handleStreaming(w http.ResponseWriter, upstreamReq *http.Request, resp *Response) {
	// Create streaming writer
	streamWriter := NewStreamingResponseWriter(w, resp, h.store)

	// Forward to scheduler
	h.schedulerHTTP.ServeHTTP(streamWriter, upstreamReq)
}

// handleNonStreaming handles non-streaming responses.
func (h *HTTPHandler) handleNonStreaming(w http.ResponseWriter, upstreamReq *http.Request, resp *Response) {
	// Capture upstream response
	capture := NewNonStreamingResponseCapture()

	// Forward to scheduler
	h.schedulerHTTP.ServeHTTP(capture, upstreamReq)

	// Check for errors
	if capture.StatusCode != http.StatusOK {
		// Try to parse error
		var errResp struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(capture.Body.String()), &errResp); err == nil && errResp.Error.Message != "" {
			resp.Status = StatusFailed
			resp.Error = &ErrorDetail{
				Code:    errResp.Error.Code,
				Message: errResp.Error.Message,
			}
			h.store.Save(resp)
			h.sendJSON(w, capture.StatusCode, resp)
			return
		}
		// Generic error
		resp.Status = StatusFailed
		resp.Error = &ErrorDetail{
			Code:    "upstream_error",
			Message: capture.Body.String(),
		}
		h.store.Save(resp)
		h.sendJSON(w, capture.StatusCode, resp)
		return
	}

	// Parse chat completion response
	var chatResp ChatCompletionResponse
	if err := json.Unmarshal([]byte(capture.Body.String()), &chatResp); err != nil {
		resp.Status = StatusFailed
		resp.Error = &ErrorDetail{
			Code:    "parse_error",
			Message: "Failed to parse upstream response",
		}
		h.store.Save(resp)
		h.sendJSON(w, http.StatusInternalServerError, resp)
		return
	}

	// Transform response
	finalResp := TransformChatCompletionToResponse(&chatResp, resp.ID, resp.Model)
	// Preserve request parameters
	finalResp.Instructions = resp.Instructions
	finalResp.Temperature = resp.Temperature
	finalResp.TopP = resp.TopP
	finalResp.MaxOutputTokens = resp.MaxOutputTokens
	finalResp.Tools = resp.Tools
	finalResp.ToolChoice = resp.ToolChoice
	finalResp.ParallelToolCalls = resp.ParallelToolCalls
	finalResp.Metadata = resp.Metadata
	finalResp.PreviousResponseID = resp.PreviousResponseID
	finalResp.ReasoningEffort = resp.ReasoningEffort
	finalResp.User = resp.User
	finalResp.CreatedAt = resp.CreatedAt

	// Store the response
	h.store.Save(finalResp)

	// Send response
	h.sendJSON(w, http.StatusOK, finalResp)
}

// handleGet handles GET /responses/{id}.
func (h *HTTPHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		h.sendError(w, http.StatusBadRequest, "invalid_request", "Response ID is required")
		return
	}

	resp, ok := h.store.Get(id)
	if !ok {
		h.sendError(w, http.StatusNotFound, "not_found", "Response not found")
		return
	}

	// Check if streaming is requested
	if r.URL.Query().Get("stream") == "true" {
		// For completed responses, we can't re-stream
		// Just return the response as JSON
		h.sendJSON(w, http.StatusOK, resp)
		return
	}

	h.sendJSON(w, http.StatusOK, resp)
}

// handleListInputItems handles GET /responses/{id}/input_items.
func (h *HTTPHandler) handleListInputItems(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		h.sendError(w, http.StatusBadRequest, "invalid_request", "Response ID is required")
		return
	}

	_, ok := h.store.Get(id)
	if !ok {
		h.sendError(w, http.StatusNotFound, "not_found", "Response not found")
		return
	}

	// For now, return an empty list since input items are not stored separately
	// In a real implementation, this would return the input items associated with the response
	h.sendJSON(w, http.StatusOK, map[string]interface{}{
		"object": "list",
		"data":   []interface{}{},
	})
}

// handleDelete handles DELETE /responses/{id}.
func (h *HTTPHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		h.sendError(w, http.StatusBadRequest, "invalid_request", "Response ID is required")
		return
	}

	if !h.store.Delete(id) {
		h.sendError(w, http.StatusNotFound, "not_found", "Response not found")
		return
	}

	h.sendJSON(w, http.StatusOK, map[string]interface{}{
		"id":      id,
		"object":  "response.deleted",
		"deleted": true,
	})
}

// sendJSON sends a JSON response.
func (h *HTTPHandler) sendJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.log.Error("Failed to encode JSON response", "error", err)
	}
}

// sendError sends an error response.
func (h *HTTPHandler) sendError(w http.ResponseWriter, statusCode int, code, message string) {
	h.sendJSON(w, statusCode, map[string]interface{}{
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	})
}

// nilIfEmpty returns a pointer to the string if non-empty, otherwise nil.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// GetStore returns the response store (for testing).
func (h *HTTPHandler) GetStore() *Store {
	return h.store
}

// SetMaxRequestBodyBytes sets the maximum request body size in bytes.
func (h *HTTPHandler) SetMaxRequestBodyBytes(bytes int64) {
	h.maxRequestBodyBytes = bytes
}

// ResponseWithTimestamp adds a timestamp helper.
func ResponseWithTimestamp(resp *Response) *Response {
	resp.CreatedAt = float64(time.Now().Unix())
	return resp
}
