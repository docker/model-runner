package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/inference/scheduling"
	"github.com/docker/model-runner/pkg/internal/utils"
	"github.com/docker/model-runner/pkg/logging"
	"github.com/docker/model-runner/pkg/middleware"
)

const (
	// APIPrefix Ollama API prefix
	APIPrefix = "/api"
)

// HTTPHandler implements the Ollama API compatibility layer
type HTTPHandler struct {
	log           logging.Logger
	router        *http.ServeMux
	httpHandler   http.Handler
	modelManager  *models.Manager
	scheduler     *scheduling.Scheduler
	schedulerHTTP http.Handler
}

// NewHTTPHandler creates a new Ollama API handler
func NewHTTPHandler(log logging.Logger, scheduler *scheduling.Scheduler, schedulerHTTP http.Handler, allowedOrigins []string, modelManager *models.Manager) *HTTPHandler {
	h := &HTTPHandler{
		log:           log,
		router:        http.NewServeMux(),
		scheduler:     scheduler,
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

// ServeHTTP implements the http.Handler interface
func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	safeMethod := utils.SanitizeForLog(r.Method, -1)
	safePath := utils.SanitizeForLog(r.URL.Path, -1)
	h.log.Infof("Ollama API request: %s %s", safeMethod, safePath)
	h.httpHandler.ServeHTTP(w, r)
}

// routeHandlers returns the mapping of routes to their handlers
func (h *HTTPHandler) routeHandlers() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"GET " + APIPrefix + "/version":   h.handleVersion,
		"GET " + APIPrefix + "/tags":      h.handleListModels,
		"GET " + APIPrefix + "/ps":        h.handlePS,
		"POST " + APIPrefix + "/show":     h.handleShowModel,
		"POST " + APIPrefix + "/chat":     h.handleChat,
		"POST " + APIPrefix + "/generate": h.handleGenerate,
		"POST " + APIPrefix + "/pull":     h.handlePull,
		"DELETE " + APIPrefix + "/delete": h.handleDelete,
	}
}

// ListResponse is the response for /api/tags
type ListResponse struct {
	Models []ModelResponse `json:"models"`
}

// ModelResponse represents a single model in the list
type ModelResponse struct {
	Name       string       `json:"name"`
	ModifiedAt time.Time    `json:"modified_at"`
	Size       int64        `json:"size"`
	Digest     string       `json:"digest"`
	Details    ModelDetails `json:"details"`
}

// ModelDetails contains model metadata
type ModelDetails struct {
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

// ShowRequest is the request for /api/show
type ShowRequest struct {
	Name    string `json:"name"`  // Ollama uses 'name' field
	Model   string `json:"model"` // Also accept 'model' for compatibility
	Verbose bool   `json:"verbose,omitempty"`
}

// ShowResponse is the response for /api/show
type ShowResponse struct {
	License    string       `json:"license,omitempty"`
	Modelfile  string       `json:"modelfile,omitempty"`
	Parameters string       `json:"parameters,omitempty"`
	Template   string       `json:"template,omitempty"`
	Details    ModelDetails `json:"details,omitempty"`
}

// ChatRequest is the request for /api/chat
type ChatRequest struct {
	Name      string                 `json:"name"`  // Ollama uses 'name' field
	Model     string                 `json:"model"` // Also accept 'model' for compatibility
	Messages  []Message              `json:"messages"`
	Stream    *bool                  `json:"stream,omitempty"`
	KeepAlive string                 `json:"keep_alive,omitempty"` // Duration like "5m" or "0s" to unload immediately
	Options   map[string]interface{} `json:"options,omitempty"`
}

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatResponse is the response for /api/chat
type ChatResponse struct {
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	Message   Message   `json:"message,omitempty"`
	Done      bool      `json:"done"`
}

// GenerateRequest is the request for /api/generate
type GenerateRequest struct {
	Name      string                 `json:"name"`  // Ollama uses 'name' field
	Model     string                 `json:"model"` // Also accept 'model' for compatibility
	Prompt    string                 `json:"prompt"`
	Stream    *bool                  `json:"stream,omitempty"`
	KeepAlive string                 `json:"keep_alive,omitempty"` // Duration like "5m" or "0s" to unload immediately
	Options   map[string]interface{} `json:"options,omitempty"`
}

// GenerateResponse is the response for /api/generate
type GenerateResponse struct {
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	Response  string    `json:"response,omitempty"`
	Done      bool      `json:"done"`
}

// DeleteRequest is the request for DELETE /api/delete
type DeleteRequest struct {
	Name  string `json:"name"`  // Ollama uses 'name' field
	Model string `json:"model"` // Also accept 'model' for compatibility
}

// PullRequest is the request for POST /api/pull
type PullRequest struct {
	Name     string `json:"name"`  // Ollama uses 'name' field
	Model    string `json:"model"` // Also accept 'model' for compatibility
	Insecure bool   `json:"insecure,omitempty"`
	Stream   *bool  `json:"stream,omitempty"`
}

// progressMessage represents the internal progress format from distribution client
type progressMessage struct {
	Type    string        `json:"type"`
	Message string        `json:"message"`
	Total   uint64        `json:"total"`
	Pulled  uint64        `json:"pulled"`
	Layer   progressLayer `json:"layer"`
}

// progressLayer represents layer information in progress messages
type progressLayer struct {
	ID      string `json:"id"`
	Size    uint64 `json:"size"`
	Current uint64 `json:"current"`
}

// ollamaPullStatus represents the Ollama pull status response format
type ollamaPullStatus struct {
	Status    string `json:"status,omitempty"`
	Digest    string `json:"digest,omitempty"`
	Total     uint64 `json:"total,omitempty"`
	Completed uint64 `json:"completed,omitempty"`
	Error     string `json:"error,omitempty"`
}

// ollamaProgressWriter wraps an http.ResponseWriter and translates
// internal progress format to ollama-compatible format
type ollamaProgressWriter struct {
	writer      http.ResponseWriter
	log         logging.Logger
	headersSent bool
}

func (w *ollamaProgressWriter) Header() http.Header {
	return w.writer.Header()
}

func (w *ollamaProgressWriter) Write(p []byte) (n int, err error) {
	// Ensure headers are sent with correct content type
	if !w.headersSent {
		w.writer.Header().Set("Content-Type", "application/x-ndjson")
		w.writer.WriteHeader(http.StatusOK)
		w.headersSent = true
	}

	// Try to parse as progress message
	var msg progressMessage
	if parseErr := json.Unmarshal(p, &msg); parseErr != nil {
		// If not JSON or doesn't match format, pass through
		return w.writer.Write(p)
	}

	// Convert to ollama format using typed struct
	var ollamaMsg ollamaPullStatus

	switch msg.Type {
	case "progress":
		// Ollama progress format for layer download
		ollamaMsg.Status = "pulling manifest"
		if msg.Layer.ID != "" {
			// Shorten digest for display (ollama uses short form)
			digest := msg.Layer.ID
			const sha256Prefix = "sha256:"
			const shortDigestLength = 12
			if len(digest) >= len(sha256Prefix)+shortDigestLength && strings.HasPrefix(digest, sha256Prefix) {
				digest = digest[len(sha256Prefix) : len(sha256Prefix)+shortDigestLength]
			}
			ollamaMsg.Status = fmt.Sprintf("pulling %s", digest)
			ollamaMsg.Digest = msg.Layer.ID
		}
		ollamaMsg.Total = msg.Layer.Size
		ollamaMsg.Completed = msg.Layer.Current

	case "success":
		ollamaMsg.Status = "success"

	case "error":
		ollamaMsg.Error = msg.Message

	case "warning":
		// Pass warnings through with a status field
		ollamaMsg.Status = msg.Message

	default:
		// Unknown message type - pass through original payload
		if msg.Type == "" {
			// Empty type, pass through
			return w.writer.Write(p)
		}
		// Unrecognized type, pass through to avoid losing information
		w.log.Warnf("Unknown progress message type: %s", msg.Type)
		return w.writer.Write(p)
	}

	// Marshal and write ollama format
	data, err := json.Marshal(ollamaMsg)
	if err != nil {
		w.log.Warnf("Failed to marshal ollama progress: %v", err)
		return w.writer.Write(p)
	}

	// Write with newline
	data = append(data, '\n')
	n, err = w.writer.Write(data)

	// Flush after each write for real-time progress
	w.Flush()

	return n, err
}

func (w *ollamaProgressWriter) WriteHeader(statusCode int) {
	if !w.headersSent {
		w.writer.WriteHeader(statusCode)
		w.headersSent = true
	}
}

func (w *ollamaProgressWriter) Flush() {
	if flusher, ok := w.writer.(http.Flusher); ok {
		flusher.Flush()
	}
}

// OpenAI API response types for type-safe parsing

// openAIChatResponse represents the OpenAI chat completion response
type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// openAIChatStreamChunk represents a chunk from OpenAI chat completion stream
type openAIChatStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

// openAIErrorResponse represents the OpenAI error response format
type openAIErrorResponse struct {
	Error struct {
		Message string      `json:"message"`
		Type    string      `json:"type"`
		Code    interface{} `json:"code"` // Can be int, string, or null
	} `json:"error"`
}

// handleVersion handles GET /api/version
func (h *HTTPHandler) handleVersion(w http.ResponseWriter, r *http.Request) {
	response := map[string]string{
		"version": "0.1.0",
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.log.Errorf("Failed to encode response: %v", err)
	}
}

// handleListModels handles GET /api/tags
func (h *HTTPHandler) handleListModels(w http.ResponseWriter, r *http.Request) {
	// Get models from the model manager
	modelsList, err := h.modelManager.List()
	if err != nil {
		h.log.Errorf("Failed to list models: %v", err)
		http.Error(w, "Failed to list models", http.StatusInternalServerError)
		return
	}

	// Convert to Ollama format
	response := ListResponse{
		Models: make([]ModelResponse, 0, len(modelsList)),
	}

	for _, model := range modelsList {
		// Extract details from the model
		details := ModelDetails{
			Format:            "gguf", // Default to gguf for now
			Family:            model.Config.Architecture,
			Families:          []string{model.Config.Architecture},
			ParameterSize:     model.Config.Parameters,
			QuantizationLevel: model.Config.Quantization,
		}

		// Get the first tag as the name, or use ID if no tags
		name := model.ID
		if len(model.Tags) > 0 {
			name = model.Tags[0]
		}

		// Parse size from config string to int64
		size := int64(0)
		// TODO: Parse size from model.Config.Size if needed

		response.Models = append(response.Models, ModelResponse{
			Name:       name,
			ModifiedAt: time.Unix(model.Created, 0),
			Size:       size,
			Digest:     model.ID,
			Details:    details,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.log.Errorf("Failed to encode response: %v", err)
	}
}

// PSModel represents a running model in the ps response
type PSModel struct {
	Name      string    `json:"name"`
	Model     string    `json:"model"`
	Size      int64     `json:"size"`
	Digest    string    `json:"digest"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	SizeVram  int64     `json:"size_vram,omitempty"`
}

// handlePS handles GET /api/ps (list running models)
func (h *HTTPHandler) handlePS(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get running backends from scheduler
	runningBackends := h.scheduler.GetRunningBackendsInfo(ctx)

	// Convert to Ollama format
	models := make([]PSModel, 0, len(runningBackends))
	for _, backend := range runningBackends {
		// Get model details to populate additional fields
		model, err := h.modelManager.GetLocal(backend.ModelName)
		if err != nil {
			h.log.Warnf("Failed to get model details for %s: %v", backend.ModelName, err)
			// Still add the model with basic info
			models = append(models, PSModel{
				Name:   backend.ModelName,
				Model:  backend.ModelName,
				Digest: backend.ModelName,
			})
			continue
		}

		// Get the first tag as the name
		name := backend.ModelName
		tags := model.Tags()
		if len(tags) > 0 {
			name = tags[0]
		}

		modelID, _ := model.ID()
		psModel := PSModel{
			Name:   name,
			Model:  name,
			Digest: modelID,
		}

		// Add expiration time if not in use
		if !backend.InUse && !backend.LastUsed.IsZero() {
			// Models typically expire 5 minutes after last use
			psModel.ExpiresAt = backend.LastUsed.Add(5 * time.Minute)
		}

		models = append(models, psModel)
	}

	response := map[string]interface{}{
		"models": models,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.log.Errorf("Failed to encode response: %v", err)
	}
}

// handleShowModel handles POST /api/show
func (h *HTTPHandler) handleShowModel(w http.ResponseWriter, r *http.Request) {
	var req ShowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Use 'name' field if present, otherwise fall back to 'model'
	modelName := req.Name
	if modelName == "" {
		modelName = req.Model
	}

	// Normalize model name
	modelName = models.NormalizeModelName(modelName)

	// Get model details
	model, err := h.modelManager.GetLocal(modelName)
	if err != nil {
		h.log.Errorf("Failed to get model: %v", err)
		http.Error(w, fmt.Sprintf("Model not found: %v", err), http.StatusNotFound)
		return
	}

	// Get config
	config, err := model.Config()
	if err != nil {
		h.log.Errorf("Failed to get model config: %v", err)
		http.Error(w, fmt.Sprintf("Failed to get model config: %v", err), http.StatusInternalServerError)
		return
	}

	// Build response
	response := ShowResponse{
		Details: ModelDetails{
			Format:            "gguf",
			Family:            config.Architecture,
			Families:          []string{config.Architecture},
			ParameterSize:     config.Parameters,
			QuantizationLevel: config.Quantization,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.log.Errorf("Failed to encode response: %v", err)
	}
}

// handleChat handles POST /api/chat
func (h *HTTPHandler) handleChat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Use 'name' field if present, otherwise fall back to 'model'
	modelName := req.Name
	if modelName == "" {
		modelName = req.Model
	}

	// Normalize model name
	modelName = models.NormalizeModelName(modelName)

	// Check if keep_alive is 0 (unload model)
	sanitizedModelName := utils.SanitizeForLog(modelName, -1)
	sanitizedKeepAlive := utils.SanitizeForLog(req.KeepAlive, -1)
	h.log.Infof("handleChat: model=%s, keep_alive=%v", sanitizedModelName, sanitizedKeepAlive)
	if req.KeepAlive == "0" || req.KeepAlive == "0s" || req.KeepAlive == "0m" {
		h.log.Infof("handleChat: unloading model %s due to keep_alive=%s", sanitizedModelName, sanitizedKeepAlive)
		h.unloadModel(ctx, w, modelName)
		return
	}

	// Handle num_ctx option for context size configuration
	if req.Options != nil {
		if numCtxRaw, ok := req.Options["num_ctx"]; ok {
			if numCtx := convertToInt64(numCtxRaw); numCtx > 0 {
				sanitizedNumCtx := utils.SanitizeForLog(fmt.Sprintf("%d", numCtx), -1)
				h.log.Infof("handleChat: configuring context size %s for model %s", sanitizedNumCtx, sanitizedModelName)
				if err := h.configureContextSize(ctx, modelName, numCtx); err != nil {
					// Log the error but continue with the request
					h.log.Warnf("handleChat: failed to configure context size for model %s: %v", sanitizedModelName, err)
				}
			}
		}
	}

	// Convert to OpenAI format chat completion request
	openAIReq := map[string]interface{}{
		"model":    modelName,
		"messages": convertMessages(req.Messages),
		"stream":   req.Stream == nil || *req.Stream,
	}

	// Add options if present
	if req.Options != nil {
		if temp, ok := req.Options["temperature"]; ok {
			openAIReq["temperature"] = temp
		}
		if maxTokens, ok := req.Options["num_predict"]; ok {
			openAIReq["max_tokens"] = maxTokens
		}
	}

	// Make request to scheduler
	h.proxyToChatCompletions(ctx, w, r, openAIReq, modelName, req.Stream == nil || *req.Stream)
}

// handleGenerate handles POST /api/generate
func (h *HTTPHandler) handleGenerate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Errorf("handleGenerate: failed to decode request: %v", err)
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Use 'name' field if present, otherwise fall back to 'model'
	modelName := req.Name
	if modelName == "" {
		modelName = req.Model
	}

	// Normalize model name
	modelName = models.NormalizeModelName(modelName)

	// Check if keep_alive is 0 (unload model)
	// Sanitize user input before logging to prevent log injection
	sanitizedModelName := utils.SanitizeForLog(modelName, -1)
	sanitizedKeepAlive := utils.SanitizeForLog(req.KeepAlive, -1)
	h.log.Infof("handleGenerate: model=%s, keep_alive=%v", sanitizedModelName, sanitizedKeepAlive)
	if req.KeepAlive == "0" || req.KeepAlive == "0s" || req.KeepAlive == "0m" {
		h.log.Infof("handleGenerate: unloading model %s due to keep_alive=%s", sanitizedModelName, sanitizedKeepAlive)
		h.unloadModel(ctx, w, modelName)
		return
	}

	// Handle num_ctx option for context size configuration
	if req.Options != nil {
		if numCtxRaw, ok := req.Options["num_ctx"]; ok {
			if numCtx := convertToInt64(numCtxRaw); numCtx > 0 {
				sanitizedNumCtx := utils.SanitizeForLog(fmt.Sprintf("%d", numCtx), -1)
				h.log.Infof("handleGenerate: configuring context size %s for model %s", sanitizedNumCtx, sanitizedModelName)
				if err := h.configureContextSize(ctx, modelName, numCtx); err != nil {
					// Log the error but continue with the request
					h.log.Warnf("handleGenerate: failed to configure context size for model %s: %v", sanitizedModelName, err)
				}
			}
		}
	}

	// Convert to OpenAI format completion request
	openAIReq := map[string]interface{}{
		"model": modelName,
		"messages": convertMessages([]Message{
			{Role: "user", Content: req.Prompt},
		}),
		"stream": req.Stream == nil || *req.Stream,
	}

	// Add options if present
	if req.Options != nil {
		if temp, ok := req.Options["temperature"]; ok {
			openAIReq["temperature"] = temp
		}
		if maxTokens, ok := req.Options["num_predict"]; ok {
			openAIReq["max_tokens"] = maxTokens
		}
	}

	// Make request to scheduler
	h.proxyToCompletions(ctx, w, r, openAIReq, modelName)
}

// configureContextSize configures the context size for a model by calling the scheduler's configure endpoint
func (h *HTTPHandler) configureContextSize(ctx context.Context, modelName string, contextSize int64) error {
	// Sanitize user input before logging to prevent log injection
	sanitizedModelName := utils.SanitizeForLog(modelName, -1)
	sanitizedContextSize := utils.SanitizeForLog(fmt.Sprintf("%d", contextSize), -1)
	h.log.Infof("configureContextSize: configuring model %s with context size %s", sanitizedModelName, sanitizedContextSize)

	// Create a configure request for the scheduler
	configureReq := map[string]interface{}{
		"model":        modelName,
		"context-size": contextSize,
	}

	// Marshal the configure request
	reqBody, err := json.Marshal(configureReq)
	if err != nil {
		return fmt.Errorf("failed to marshal configure request: %w", err)
	}

	// Create a new request to the scheduler
	newReq, err := http.NewRequestWithContext(ctx, http.MethodPost, inference.InferencePrefix+"/_configure", strings.NewReader(string(reqBody)))
	if err != nil {
		return fmt.Errorf("failed to create configure request: %w", err)
	}
	newReq.Header.Set("Content-Type", "application/json")

	// Use a custom response writer to capture the response
	respRecorder := &responseRecorder{
		statusCode: http.StatusOK,
		headers:    make(http.Header),
		body:       &strings.Builder{},
	}

	// Forward to scheduler HTTP handler
	h.schedulerHTTP.ServeHTTP(respRecorder, newReq)

	if respRecorder.statusCode != http.StatusOK {
		return fmt.Errorf("configure request failed with status %d: %s", respRecorder.statusCode, respRecorder.body.String())
	}

	h.log.Infof("configureContextSize: successfully configured context size for model %s", sanitizedModelName)
	return nil
}

// unloadModel unloads a model from memory
func (h *HTTPHandler) unloadModel(ctx context.Context, w http.ResponseWriter, modelName string) {
	// Sanitize user input before logging to prevent log injection
	sanitizedModelName := utils.SanitizeForLog(modelName, -1)
	h.log.Infof("unloadModel: unloading model %s", sanitizedModelName)

	// Create an unload request for the scheduler
	unloadReq := map[string]interface{}{
		"models": []string{modelName},
	}

	// Marshal the unload request
	reqBody, err := json.Marshal(unloadReq)
	if err != nil {
		h.log.Errorf("unloadModel: failed to marshal request: %v", err)
		http.Error(w, fmt.Sprintf("Failed to marshal request: %v", err), http.StatusInternalServerError)
		return
	}

	// Sanitize the user-provided request body before logging to avoid log injection
	safeReqBody := utils.SanitizeForLog(string(reqBody), -1)
	h.log.Infof("unloadModel: sending POST /engines/unload with body: %s", safeReqBody)

	// Create a new request to the scheduler
	newReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "/engines/unload", strings.NewReader(string(reqBody)))
	if err != nil {
		h.log.Errorf("unloadModel: failed to create request: %v", err)
		http.Error(w, fmt.Sprintf("Failed to create request: %v", err), http.StatusInternalServerError)
		return
	}
	newReq.Header.Set("Content-Type", "application/json")

	// Use a custom response writer to capture the response
	respRecorder := &responseRecorder{
		statusCode: http.StatusOK,
		headers:    make(http.Header),
		body:       &strings.Builder{},
	}

	// Forward to scheduler HTTP handler
	h.schedulerHTTP.ServeHTTP(respRecorder, newReq)

	h.log.Infof("unloadModel: scheduler response status=%d, body=%s", respRecorder.statusCode, respRecorder.body.String())

	// Return the response status
	w.WriteHeader(respRecorder.statusCode)
	if respRecorder.statusCode == http.StatusOK {
		// Return empty JSON object for success
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{}"))
	} else {
		w.Write([]byte(respRecorder.body.String()))
	}
}

// handleDelete handles DELETE /api/delete
func (h *HTTPHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req DeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Use 'name' field if present, otherwise fall back to 'model'
	modelName := req.Name
	if modelName == "" {
		modelName = req.Model
	}

	// Normalize model name
	modelName = models.NormalizeModelName(modelName)

	sanitizedModelName := utils.SanitizeForLog(modelName, -1)
	h.log.Infof("handleDelete: deleting model %s", sanitizedModelName)

	// First, unload the model from memory
	unloadReq := map[string]interface{}{
		"models": []string{modelName},
	}

	reqBody, err := json.Marshal(unloadReq)
	if err != nil {
		h.log.Errorf("handleDelete: failed to marshal unload request: %v", err)
		http.Error(w, fmt.Sprintf("Failed to marshal request: %v", err), http.StatusInternalServerError)
		return
	}

	newReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "/engines/unload", strings.NewReader(string(reqBody)))
	if err != nil {
		h.log.Errorf("handleDelete: failed to create unload request: %v", err)
		http.Error(w, fmt.Sprintf("Failed to create request: %v", err), http.StatusInternalServerError)
		return
	}
	newReq.Header.Set("Content-Type", "application/json")

	respRecorder := &responseRecorder{
		statusCode: http.StatusOK,
		headers:    make(http.Header),
		body:       &strings.Builder{},
	}

	h.schedulerHTTP.ServeHTTP(respRecorder, newReq)
	h.log.Infof("handleDelete: unload response status=%d", respRecorder.statusCode)

	// Check if unload succeeded before deleting from storage
	if respRecorder.statusCode < 200 || respRecorder.statusCode >= 300 {
		sanitizedBody := utils.SanitizeForLog(respRecorder.body.String(), -1)
		h.log.Errorf(
			"handleDelete: unload failed for model %s with status=%d, body=%q",
			sanitizedModelName,
			respRecorder.statusCode,
			sanitizedBody,
		)
		http.Error(
			w,
			fmt.Sprintf("Failed to unload model: scheduler returned status %d", respRecorder.statusCode),
			respRecorder.statusCode,
		)
		return
	}

	// Then delete the model from storage
	if _, err := h.modelManager.Delete(modelName, false); err != nil {
		sanitizedErr := utils.SanitizeForLog(err.Error(), -1)
		h.log.Errorf("handleDelete: failed to delete model %s: %v", sanitizedModelName, sanitizedErr)
		http.Error(w, fmt.Sprintf("Failed to delete model: %v", sanitizedErr), http.StatusInternalServerError)
		return
	}

	h.log.Infof("handleDelete: successfully deleted model %s", sanitizedModelName)

	// Return success response in Ollama format (empty JSON object)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("{}"))
}

// handlePull handles POST /api/pull
func (h *HTTPHandler) handlePull(w http.ResponseWriter, r *http.Request) {
	var req PullRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Use 'name' field if present, otherwise fall back to 'model'
	modelName := req.Name
	if modelName == "" {
		modelName = req.Model
	}

	// Normalize model name
	modelName = models.NormalizeModelName(modelName)

	// Set Accept header for JSON response (Ollama expects JSON streaming)
	r.Header.Set("Accept", "application/json")

	// Wrap the response writer with ollama progress adapter
	ollamaWriter := &ollamaProgressWriter{
		writer:      w,
		log:         h.log,
		headersSent: false,
	}

	// Call the model manager's Pull method with the wrapped writer
	if err := h.modelManager.Pull(modelName, "", r, ollamaWriter); err != nil {
		h.log.Errorf("Failed to pull model: %v", err)

		// Send error in Ollama JSON format
		errorResponse := ollamaPullStatus{
			Error: fmt.Sprintf("Failed to pull model: %v", err),
		}

		if !ollamaWriter.headersSent {
			// Headers not sent yet - we can still use http.Error
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			if err := json.NewEncoder(w).Encode(errorResponse); err != nil {
				h.log.Errorf("failed to encode response: %v", err)
			}
		} else {
			// Headers already sent - write error as JSON line
			if data, marshalErr := json.Marshal(errorResponse); marshalErr == nil {
				w.Write(data)
				w.Write([]byte("\n"))
			}
		}
	}
}

// convertMessages converts Ollama messages to OpenAI format
func convertMessages(messages []Message) []map[string]interface{} {
	result := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		result[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}
	return result
}

// convertToInt64 converts various numeric types to int64
func convertToInt64(v interface{}) int64 {
	switch val := v.(type) {
	case int:
		return int64(val)
	case int64:
		return val
	case float64:
		return int64(val)
	case float32:
		return int64(val)
	case string:
		// Sanitize string to remove newline/carriage return before parsing
		safeVal := utils.SanitizeForLog(val, -1)
		if num, err := fmt.Sscanf(safeVal, "%d", new(int64)); err == nil && num == 1 {
			var result int64
			fmt.Sscanf(safeVal, "%d", &result)
			return result
		}
	}
	return 0
}

// proxyToChatCompletions proxies the request to the OpenAI chat completions endpoint
func (h *HTTPHandler) proxyToChatCompletions(ctx context.Context, w http.ResponseWriter, r *http.Request, openAIReq map[string]interface{}, modelName string, stream bool) {
	// Marshal the OpenAI request
	reqBody, err := json.Marshal(openAIReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to marshal request: %v", err), http.StatusInternalServerError)
		return
	}

	// Clone the original request to preserve headers (User-Agent, auth, etc.)
	newReq := r.Clone(ctx)
	newReq.URL.Path = "/engines/v1/chat/completions"
	newReq.Body = io.NopCloser(bytes.NewReader(reqBody))
	newReq.ContentLength = int64(len(reqBody))
	newReq.Header.Set("Content-Type", "application/json")
	newReq.Header.Set(inference.RequestOriginHeader, inference.OriginOllamaCompletion)

	if stream {
		// Use streaming response writer that processes SSE on the fly
		streamWriter := &streamingChatResponseWriter{
			w:         w,
			modelName: modelName,
			log:       h.log,
		}
		// Forward to scheduler HTTP handler with streaming writer
		h.schedulerHTTP.ServeHTTP(streamWriter, newReq)
		return
	}

	// For non-streaming, use a response recorder to capture the response
	respRecorder := &responseRecorder{
		statusCode: http.StatusOK,
		headers:    make(http.Header),
		body:       &strings.Builder{},
	}

	// Forward to scheduler HTTP handler
	h.schedulerHTTP.ServeHTTP(respRecorder, newReq)

	// Convert non-streaming response
	h.convertChatResponse(w, respRecorder, modelName)
}

// proxyToCompletions proxies the request to the OpenAI completions endpoint
func (h *HTTPHandler) proxyToCompletions(ctx context.Context, w http.ResponseWriter, r *http.Request, openAIReq map[string]interface{}, modelName string) {
	// Marshal the OpenAI request
	reqBody, err := json.Marshal(openAIReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to marshal request: %v", err), http.StatusInternalServerError)
		return
	}

	// Clone the original request to preserve headers (User-Agent, auth, etc.)
	newReq := r.Clone(ctx)
	newReq.URL.Path = "/engines/v1/chat/completions"
	newReq.Body = io.NopCloser(bytes.NewReader(reqBody))
	newReq.ContentLength = int64(len(reqBody))
	newReq.Header.Set("Content-Type", "application/json")
	newReq.Header.Set(inference.RequestOriginHeader, inference.OriginOllamaCompletion)

	if stream, ok := openAIReq["stream"].(bool); ok && stream {
		// Use streaming response writer that processes SSE on the fly
		streamWriter := &streamingGenerateResponseWriter{
			w:         w,
			modelName: modelName,
			log:       h.log,
		}
		// Forward to scheduler HTTP handler with streaming writer
		h.schedulerHTTP.ServeHTTP(streamWriter, newReq)
		return
	}

	// For non-streaming, use a response recorder to capture the response
	respRecorder := &responseRecorder{
		statusCode: http.StatusOK,
		headers:    make(http.Header),
		body:       &strings.Builder{},
	}

	// Forward to scheduler HTTP handler
	h.schedulerHTTP.ServeHTTP(respRecorder, newReq)

	// Convert non-streaming response
	h.convertGenerateResponse(w, respRecorder, modelName)
}

// responseRecorder is a custom ResponseWriter that records the response
type responseRecorder struct {
	statusCode int
	headers    http.Header
	body       *strings.Builder
}

func (rr *responseRecorder) Header() http.Header {
	return rr.headers
}

func (rr *responseRecorder) Write(data []byte) (int, error) {
	return rr.body.Write(data)
}

func (rr *responseRecorder) WriteHeader(statusCode int) {
	rr.statusCode = statusCode
}

// streamingChatResponseWriter is a custom ResponseWriter that converts OpenAI chat SSE to Ollama format on the fly
type streamingChatResponseWriter struct {
	w           http.ResponseWriter
	modelName   string
	log         logging.Logger
	buffer      strings.Builder
	headersSent bool
}

func (s *streamingChatResponseWriter) Header() http.Header {
	return s.w.Header()
}

func (s *streamingChatResponseWriter) WriteHeader(statusCode int) {
	s.headersSent = true
	if statusCode != http.StatusOK {
		// Pass through non-success status codes
		s.w.WriteHeader(statusCode)
		return
	}
	// Set headers for Ollama streaming
	s.w.Header().Set("Content-Type", "application/json")
	s.w.Header().Set("Transfer-Encoding", "chunked")
	s.w.WriteHeader(statusCode)
}

func (s *streamingChatResponseWriter) Write(data []byte) (int, error) {
	if !s.headersSent {
		s.WriteHeader(http.StatusOK)
	}

	// Add data to buffer
	s.buffer.Write(data)

	// Process complete lines from buffer
	bufferStr := s.buffer.String()
	lines := strings.Split(bufferStr, "\n")

	// Keep the last incomplete line in the buffer
	if len(lines) > 0 && !strings.HasSuffix(bufferStr, "\n") {
		s.buffer.Reset()
		s.buffer.WriteString(lines[len(lines)-1])
		lines = lines[:len(lines)-1]
	} else {
		s.buffer.Reset()
	}

	// Process complete lines
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		dataStr := strings.TrimPrefix(line, "data: ")
		if dataStr == "[DONE]" {
			// Send final done message
			finalResp := ChatResponse{
				Model:     s.modelName,
				CreatedAt: time.Now(),
				Done:      true,
			}
			if jsonData, err := json.Marshal(finalResp); err == nil {
				s.w.Write(jsonData)
				s.w.Write([]byte("\n"))
			}
			continue
		}

		// Parse OpenAI chunk using proper struct
		var chunk openAIChatStreamChunk
		if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
			s.log.Warnf("Failed to parse OpenAI chat stream chunk: %v", err)
			continue
		}

		// Extract content from structured response
		var content string
		if len(chunk.Choices) > 0 {
			content = chunk.Choices[0].Delta.Content
		}

		// Build Ollama chunk
		ollamaChunk := ChatResponse{
			Model:     s.modelName,
			CreatedAt: time.Now(),
			Message: Message{
				Role:    "assistant",
				Content: content,
			},
			Done: false,
		}

		if jsonData, err := json.Marshal(ollamaChunk); err == nil {
			s.w.Write(jsonData)
			s.w.Write([]byte("\n"))
		}
	}

	// Flush if possible
	if flusher, ok := s.w.(http.Flusher); ok {
		flusher.Flush()
	}

	return len(data), nil
}

// streamingGenerateResponseWriter is a custom ResponseWriter that converts OpenAI completion SSE to Ollama format on the fly
type streamingGenerateResponseWriter struct {
	w           http.ResponseWriter
	modelName   string
	log         logging.Logger
	buffer      strings.Builder
	headersSent bool
}

func (s *streamingGenerateResponseWriter) Header() http.Header {
	return s.w.Header()
}

func (s *streamingGenerateResponseWriter) WriteHeader(statusCode int) {
	s.headersSent = true
	if statusCode != http.StatusOK {
		// Pass through non-success status codes
		s.w.WriteHeader(statusCode)
		return
	}
	// Set headers for Ollama streaming
	s.w.Header().Set("Content-Type", "application/json")
	s.w.Header().Set("Transfer-Encoding", "chunked")
	s.w.WriteHeader(statusCode)
}

func (s *streamingGenerateResponseWriter) Write(data []byte) (int, error) {
	if !s.headersSent {
		s.WriteHeader(http.StatusOK)
	}

	// Add data to buffer
	s.buffer.Write(data)

	// Process complete lines from buffer
	bufferStr := s.buffer.String()
	lines := strings.Split(bufferStr, "\n")

	// Keep the last incomplete line in the buffer
	if len(lines) > 0 && !strings.HasSuffix(bufferStr, "\n") {
		s.buffer.Reset()
		s.buffer.WriteString(lines[len(lines)-1])
		lines = lines[:len(lines)-1]
	} else {
		s.buffer.Reset()
	}

	// Process complete lines
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		dataStr := strings.TrimPrefix(line, "data: ")
		if dataStr == "[DONE]" {
			// Send final done message
			finalResp := GenerateResponse{
				Model:     s.modelName,
				CreatedAt: time.Now(),
				Done:      true,
			}
			if jsonData, err := json.Marshal(finalResp); err == nil {
				s.w.Write(jsonData)
				s.w.Write([]byte("\n"))
			}
			continue
		}

		// Parse OpenAI chunk using proper struct
		var chunk openAIChatStreamChunk
		if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
			s.log.Warnf("Failed to parse OpenAI chat stream chunk: %v", err)
			continue
		}

		// Extract content from structured response
		var content string
		if len(chunk.Choices) > 0 {
			content = chunk.Choices[0].Delta.Content
		}

		// Build Ollama generate chunk
		ollamaChunk := GenerateResponse{
			Model:     s.modelName,
			CreatedAt: time.Now(),
			Response:  content,
			Done:      false,
		}

		if jsonData, err := json.Marshal(ollamaChunk); err == nil {
			s.w.Write(jsonData)
			s.w.Write([]byte("\n"))
		}
	}

	// Flush if possible
	if flusher, ok := s.w.(http.Flusher); ok {
		flusher.Flush()
	}

	return len(data), nil
}

// convertChatResponse converts OpenAI chat completion response to Ollama format
func (h *HTTPHandler) convertChatResponse(w http.ResponseWriter, respRecorder *responseRecorder, modelName string) {
	// Handle error responses by converting OpenAI format to Ollama format
	if respRecorder.statusCode != http.StatusOK {
		w.WriteHeader(respRecorder.statusCode)

		// Try to parse OpenAI error format
		var openAIErr openAIErrorResponse
		if err := json.Unmarshal([]byte(respRecorder.body.String()), &openAIErr); err == nil && openAIErr.Error.Message != "" {
			// Convert to Ollama error format (simple string)
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]string{"error": openAIErr.Error.Message}); err != nil {
				h.log.Errorf("failed to encode response: %v", err)
			}
		} else {
			// Fallback: return raw error body
			w.Write([]byte(respRecorder.body.String()))
		}
		return
	}

	// Parse OpenAI response using proper struct
	var openAIResp openAIChatResponse
	if err := json.Unmarshal([]byte(respRecorder.body.String()), &openAIResp); err != nil {
		h.log.Errorf("Failed to parse OpenAI response: %v", err)
		http.Error(w, "Failed to parse response", http.StatusInternalServerError)
		return
	}

	// Extract the message content from structured response
	var content string
	if len(openAIResp.Choices) > 0 {
		content = openAIResp.Choices[0].Message.Content
	}

	// Build Ollama response
	response := ChatResponse{
		Model:     modelName,
		CreatedAt: time.Now(),
		Message: Message{
			Role:    "assistant",
			Content: content,
		},
		Done: true,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.log.Errorf("Failed to encode response: %v", err)
	}
}

// convertGenerateResponse converts OpenAI chat completion response to Ollama generate format
func (h *HTTPHandler) convertGenerateResponse(w http.ResponseWriter, respRecorder *responseRecorder, modelName string) {
	// Handle error responses by converting OpenAI format to Ollama format
	if respRecorder.statusCode != http.StatusOK {
		w.WriteHeader(respRecorder.statusCode)

		// Try to parse OpenAI error format
		var openAIErr openAIErrorResponse
		if err := json.Unmarshal([]byte(respRecorder.body.String()), &openAIErr); err == nil && openAIErr.Error.Message != "" {
			// Convert to Ollama error format (simple string)
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]string{"error": openAIErr.Error.Message}); err != nil {
				h.log.Errorf("failed to encode response: %v", err)
			}
		} else {
			// Fallback: return raw error body
			w.Write([]byte(respRecorder.body.String()))
		}
		return
	}

	// Parse OpenAI chat response (since we're now using chat completions endpoint)
	var openAIResp openAIChatResponse
	if err := json.Unmarshal([]byte(respRecorder.body.String()), &openAIResp); err != nil {
		h.log.Errorf("Failed to parse OpenAI chat response: %v", err)
		http.Error(w, "Failed to parse response", http.StatusInternalServerError)
		return
	}

	// Extract the message content from structured response
	var content string
	if len(openAIResp.Choices) > 0 {
		content = openAIResp.Choices[0].Message.Content
	}

	// Build Ollama generate response
	response := GenerateResponse{
		Model:     modelName,
		CreatedAt: time.Now(),
		Response:  content,
		Done:      true,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.log.Errorf("Failed to encode response: %v", err)
	}
}
