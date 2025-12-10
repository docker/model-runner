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

// Reasoning budget constants for the think parameter conversion
const (
	// reasoningBudgetUnlimited represents unlimited reasoning tokens (-1 for llama.cpp)
	reasoningBudgetUnlimited int32 = -1
	// reasoningBudgetDisabled disables reasoning (0 tokens)
	reasoningBudgetDisabled int32 = 0
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

		// Parse size from config string to int64
		size := int64(0)
		// TODO: Parse size from model.Config.Size if needed

		// Get tags, or use ID if no tags exist
		tags := model.Tags
		if len(tags) == 0 {
			tags = []string{model.ID}
		}

		// Create a response entry for each tag to match Ollama's behavior
		// This ensures that models with multiple tags (e.g., mymodel:latest and mymodel:v1.0)
		// are listed separately, allowing clients like OpenWebUI to see all available tags
		for _, tag := range tags {
			response.Models = append(response.Models, ModelResponse{
				Name:       tag,
				Model:      tag,
				ModifiedAt: time.Unix(model.Created, 0),
				Size:       size,
				Digest:     model.ID,
				Details:    details,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.log.Errorf("Failed to encode response: %v", err)
	}
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

	// Configure model
	h.configureModel(ctx, modelName, req.Options, req.Think, r.UserAgent()+" (Ollama API)")

	// Convert to OpenAI format chat completion request
	openAIReq := map[string]interface{}{
		"model":    modelName,
		"messages": convertMessages(req.Messages),
		"stream":   req.Stream == nil || *req.Stream,
	}

	// Add tools if present
	if len(req.Tools) > 0 {
		openAIReq["tools"] = req.Tools
	}

	// Map Ollama options to OpenAI format
	if req.Options != nil {
		h.mapOllamaOptionsToOpenAI(req.Options, openAIReq)
	}

	// Make request to scheduler
	h.proxyToChatCompletions(ctx, w, r, openAIReq, modelName, req.Stream == nil || *req.Stream)
}

// configureModel extracts and applies model configuration options.
// Handles num_ctx from options and think parameter for reasoning budget.
func (h *HTTPHandler) configureModel(ctx context.Context, modelName string, options map[string]interface{}, think interface{}, userAgent string) {
	var contextSize int32
	var hasContextSize bool

	// Extract context size from options
	if options != nil {
		if numCtxRaw, ok := options["num_ctx"]; ok && numCtxRaw != nil {
			contextSize = convertToInt32(numCtxRaw)
			hasContextSize = true
		}
	}

	// Convert think parameter to --reasoning-budget flag (returns nil if not specified)
	reasoningBudget := convertThinkToReasoningBudget(think)

	// Only call ConfigureRunner if we have something to configure
	if hasContextSize || reasoningBudget != nil {
		sanitizedModelName := utils.SanitizeForLog(modelName, -1)
		h.log.Infof("configureModel: configuring model %s", sanitizedModelName)
		configureRequest := scheduling.ConfigureRequest{
			Model: modelName,
		}
		// Only include ContextSize if explicitly defined
		if hasContextSize {
			configureRequest.ContextSize = &contextSize
		}
		// Set llama.cpp-specific reasoning budget if provided
		if reasoningBudget != nil {
			configureRequest.LlamaCpp = &inference.LlamaCppConfig{
				ReasoningBudget: reasoningBudget,
			}
		}
		_, err := h.scheduler.ConfigureRunner(ctx, nil, configureRequest, userAgent) // TODO add backend selection?
		if err != nil {
			// Log the error but continue with the request
			h.log.Warnf("configureModel: failed to configure model %s: %v", sanitizedModelName, err)
		}
	}
}

// isZeroKeepAlive checks if the keep-alive duration string represents zero duration.
// Returns true for "0", "0s", "0m", or empty string.
func isZeroKeepAlive(keepAlive string) bool {
	return keepAlive == "0" || keepAlive == "0s" || keepAlive == "0m" || keepAlive == ""
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

	if req.Prompt == "" && isZeroKeepAlive(req.KeepAlive) {
		h.unloadModel(ctx, w, modelName)
		return
	}

	// Configure model
	h.configureModel(ctx, modelName, req.Options, req.Think, r.UserAgent()+" (Ollama API)")

	if req.Prompt == "" {
		// Empty prompt - preload the model (already configured above)
		// Return success response in Ollama format (empty JSON object)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
		return
	}

	// Convert to OpenAI format completion request
	openAIReq := map[string]interface{}{
		"model": modelName,
		"messages": convertMessages([]Message{
			{Role: "user", Content: req.Prompt},
		}),
		"stream": req.Stream == nil || *req.Stream,
	}

	// Map Ollama options to OpenAI format
	if req.Options != nil {
		h.mapOllamaOptionsToOpenAI(req.Options, openAIReq)
	}

	// Make request to scheduler
	h.proxyToCompletions(ctx, w, r, openAIReq, modelName)
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

// mapOllamaOptionsToOpenAI maps Ollama API options to OpenAI-compatible format
// This function handles all standard Ollama options and maps them to their OpenAI equivalents
func (h *HTTPHandler) mapOllamaOptionsToOpenAI(ollamaOpts map[string]interface{}, openAIReq map[string]interface{}) {
	// Direct mappings - these options have exact OpenAI equivalents
	if val, ok := ollamaOpts["temperature"]; ok {
		openAIReq["temperature"] = val
	}
	if val, ok := ollamaOpts["top_p"]; ok {
		openAIReq["top_p"] = val
	}
	if val, ok := ollamaOpts["top_k"]; ok {
		openAIReq["top_k"] = val
	}
	if val, ok := ollamaOpts["num_predict"]; ok {
		openAIReq["max_tokens"] = val
	}
	if val, ok := ollamaOpts["stop"]; ok {
		openAIReq["stop"] = val
	}
	if val, ok := ollamaOpts["seed"]; ok {
		openAIReq["seed"] = val
	}
	if val, ok := ollamaOpts["presence_penalty"]; ok {
		openAIReq["presence_penalty"] = val
	}
	if val, ok := ollamaOpts["frequency_penalty"]; ok {
		openAIReq["frequency_penalty"] = val
	}

	// Note: num_ctx is handled separately in the configure() function
	// as it requires a special ConfigureRunner call
}

// ensureDataURIPrefix ensures that image data has a proper data URI prefix.
// OpenWebUI may send raw base64 data without prefix, but llama.cpp requires it.
// This function:
// - Returns data as-is if it already starts with "data:", "http://", or "https://"
// - Detects MIME type from base64 prefix and prepends appropriate data URI
func ensureDataURIPrefix(imageData string) string {
	// Trim whitespace that might come from UIs
	imageData = strings.TrimSpace(imageData)

	// Check if already has a URI scheme
	if strings.HasPrefix(imageData, "data:") ||
		strings.HasPrefix(imageData, "http://") ||
		strings.HasPrefix(imageData, "https://") {
		return imageData
	}

	// Detect MIME type from base64 prefix
	var mimeType string
	if strings.HasPrefix(imageData, "/9j/") {
		mimeType = "image/jpeg"
	} else if strings.HasPrefix(imageData, "iVBOR") {
		mimeType = "image/png"
	} else if strings.HasPrefix(imageData, "R0lG") {
		mimeType = "image/gif"
	} else {
		// Default to jpeg for unknown formats
		mimeType = "image/jpeg"
	}

	// Assume raw base64 data - add data URI prefix with detected MIME type
	return "data:" + mimeType + ";base64," + imageData
}

// convertMessages converts Ollama messages to OpenAI format
func convertMessages(messages []Message) []map[string]interface{} {
	result := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		openAIMsg := map[string]interface{}{
			"role": msg.Role,
		}

		// Handle multimodal content (text + images)
		if len(msg.Images) > 0 {
			// Convert to OpenAI multimodal format: content is an array of content objects
			contentArraySize := len(msg.Images)
			if msg.Content != "" {
				contentArraySize++
			}
			contentArray := make([]map[string]interface{}, 0, contentArraySize)

			// Add text content if present
			if msg.Content != "" {
				contentArray = append(contentArray, map[string]interface{}{
					"type": "text",
					"text": msg.Content,
				})
			}

			// Add images in OpenAI format
			for _, imageData := range msg.Images {
				// Ensure image data has proper data URI prefix
				// OpenWebUI may send raw base64 without the prefix, but llama.cpp requires it
				imageURL := ensureDataURIPrefix(imageData)

				contentArray = append(contentArray, map[string]interface{}{
					"type": "image_url",
					"image_url": map[string]interface{}{
						"url": imageURL,
					},
				})
			}

			openAIMsg["content"] = contentArray
		} else {
			// Regular text-only message
			openAIMsg["content"] = msg.Content
		}

		// Add tool calls if present (for assistant messages)
		if len(msg.ToolCalls) > 0 {
			// Ensure type field is set and arguments are JSON strings for OpenAI compatibility
			toolCalls := make([]ToolCall, len(msg.ToolCalls))
			for i, tc := range msg.ToolCalls {
				toolCalls[i] = tc
				if toolCalls[i].Type == "" {
					toolCalls[i].Type = "function"
				}
				// Convert arguments to JSON string if it's an object
				if argsObj, ok := tc.Function.Arguments.(map[string]interface{}); ok {
					if argsJSON, err := json.Marshal(argsObj); err == nil {
						toolCalls[i].Function.Arguments = string(argsJSON)
					}
				}
			}
			openAIMsg["tool_calls"] = toolCalls
		}

		// Add tool_call_id if present (for tool result messages)
		if msg.ToolCallID != "" {
			openAIMsg["tool_call_id"] = msg.ToolCallID
		}

		result[i] = openAIMsg
	}
	return result
}

// convertThinkToReasoningBudget converts the Ollama 'think' parameter to llama.cpp's 'reasoning_budget'.
// The think parameter must be a boolean:
// - true: unlimited reasoning (-1)
// - false: reasoning disabled (0)
// Returns nil if think is nil or not a boolean.
func convertThinkToReasoningBudget(think interface{}) *int32 {
	if think == nil {
		return nil
	}

	// Helper to create a pointer to an int32 value
	ptr := func(v int32) *int32 { return &v }

	// Only accept boolean values
	if v, ok := think.(bool); ok {
		if v {
			return ptr(reasoningBudgetUnlimited)
		}
		return ptr(reasoningBudgetDisabled)
	}

	// Invalid type - return nil
	return nil
}

// convertToInt32 converts various numeric types to int32
func convertToInt32(v interface{}) int32 {
	switch val := v.(type) {
	case int:
		return int32(val)
	case int32:
		return val
	case float64:
		return int32(val)
	case float32:
		return int32(val)
	case string:
		// Sanitize string to remove newline/carriage return before parsing
		safeVal := utils.SanitizeForLog(val, -1)
		if num, err := fmt.Sscanf(safeVal, "%d", new(int32)); err == nil && num == 1 {
			var result int32
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

		// Extract content, tool calls, and thinking from structured response
		var content string
		var thinking string
		var toolCalls []ToolCall
		if len(chunk.Choices) > 0 {
			content = chunk.Choices[0].Delta.Content
			thinking = chunk.Choices[0].Delta.ReasoningContent
			if len(chunk.Choices[0].Delta.ToolCalls) > 0 {
				// Convert tool calls to Ollama format
				toolCalls = convertToolCallsToOllamaFormat(chunk.Choices[0].Delta.ToolCalls)
			}
		}

		// Build Ollama chunk
		message := Message{
			Role:    "assistant",
			Content: content,
		}
		if len(toolCalls) > 0 {
			message.ToolCalls = toolCalls
		}
		if thinking != "" {
			message.Thinking = thinking
		}

		ollamaChunk := ChatResponse{
			Model:     s.modelName,
			CreatedAt: time.Now(),
			Message:   message,
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

		// Extract content and reasoning_content from structured response
		var content string
		var thinking string
		if len(chunk.Choices) > 0 {
			content = chunk.Choices[0].Delta.Content
			thinking = chunk.Choices[0].Delta.ReasoningContent
		}

		// Build Ollama generate chunk
		ollamaChunk := GenerateResponse{
			Model:     s.modelName,
			CreatedAt: time.Now(),
			Response:  content,
			Thinking:  thinking,
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

	// Extract the message from structured response
	var message Message
	if len(openAIResp.Choices) > 0 {
		message.Role = "assistant"
		message.Content = openAIResp.Choices[0].Message.Content
		// Include tool calls if present
		if len(openAIResp.Choices[0].Message.ToolCalls) > 0 {
			message.ToolCalls = convertToolCallsToOllamaFormat(openAIResp.Choices[0].Message.ToolCalls)
		}
		// Include thinking content if present
		if openAIResp.Choices[0].Message.ReasoningContent != "" {
			message.Thinking = openAIResp.Choices[0].Message.ReasoningContent
		}
	}

	// Build Ollama response
	response := ChatResponse{
		Model:     modelName,
		CreatedAt: time.Now(),
		Message:   message,
		Done:      true,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.log.Errorf("Failed to encode response: %v", err)
	}
}

// convertToolCallsToOllamaFormat converts tool calls from OpenAI format to Ollama format
// This parses the arguments from JSON string to object and adds index field
func convertToolCallsToOllamaFormat(toolCalls []ToolCall) []ToolCall {
	if len(toolCalls) == 0 {
		return toolCalls
	}

	converted := make([]ToolCall, len(toolCalls))
	for i, tc := range toolCalls {
		converted[i] = tc
		// Remove type field for Ollama compatibility (it will be omitted in JSON if empty)
		converted[i].Type = ""

		// Parse arguments from JSON string to object for Ollama compatibility
		if argsStr, ok := tc.Function.Arguments.(string); ok && argsStr != "" {
			var argsObj map[string]interface{}
			if err := json.Unmarshal([]byte(argsStr), &argsObj); err == nil {
				converted[i].Function.Arguments = argsObj
			}
		}

		// Add index field
		index := i
		converted[i].Function.Index = &index
	}

	return converted
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

	// Extract the message content and reasoning content from structured response
	var content string
	var thinking string
	if len(openAIResp.Choices) > 0 {
		content = openAIResp.Choices[0].Message.Content
		thinking = openAIResp.Choices[0].Message.ReasoningContent
	}

	// Build Ollama generate response
	response := GenerateResponse{
		Model:     modelName,
		CreatedAt: time.Now(),
		Response:  content,
		Thinking:  thinking,
		Done:      true,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.log.Errorf("Failed to encode response: %v", err)
	}
}
