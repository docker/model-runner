package scheduling

import (
	"strings"
	"time"

	"github.com/docker/model-runner/pkg/inference"
)

const (
	// maximumOpenAIInferenceRequestSize is the maximum OpenAI API embedding or
	// completion request size that Scheduler will allow. This should be large
	// enough to encompass any real-world request but also small enough to avoid
	// DoS attacks.
	maximumOpenAIInferenceRequestSize = 10 * 1024 * 1024
)

// trimRequestPathToOpenAIRoot trims a request path to start at the first
// instance of /v1/ to appear in the path.
func trimRequestPathToOpenAIRoot(path string) string {
	if index := strings.Index(path, "/v1/"); index != -1 {
		return path[index:]
	} else if index = strings.Index(path, "/rerank"); index != -1 {
		return path[index:]
	} else if index = strings.Index(path, "/score"); index != -1 {
		return path[index:]
	}
	return path
}

// backendModeForRequest determines the backend operation mode to handle an
// OpenAI or Anthropic inference request. Its second parameter is true if and only if a valid
// mode could be determined.
func backendModeForRequest(path string) (inference.BackendMode, bool) {
	if strings.HasSuffix(path, "/v1/chat/completions") || strings.HasSuffix(path, "/v1/completions") {
		return inference.BackendModeCompletion, true
	} else if strings.HasSuffix(path, "/v1/embeddings") {
		return inference.BackendModeEmbedding, true
	} else if strings.HasSuffix(path, "/rerank") || strings.HasSuffix(path, "/score") {
		return inference.BackendModeReranking, true
	} else if strings.HasSuffix(path, "/v1/messages") || strings.HasSuffix(path, "/v1/messages/count_tokens") {
		// Anthropic Messages API - treated as completion mode
		return inference.BackendModeCompletion, true
	} else if strings.HasSuffix(path, "/v1/images/generations") {
		// OpenAI Images API - image generation mode
		return inference.BackendModeImageGeneration, true
	}
	return inference.BackendMode(0), false
}

// OpenAIInferenceRequest is used to extract the model specification from either
// a chat completion or embedding request in the OpenAI API.
type OpenAIInferenceRequest struct {
	// Model is the requested model name.
	Model string `json:"model"`
}

// OpenAIErrorResponse is used to format an OpenAI API compatible error response
// (see https://platform.openai.com/docs/api-reference/responses-streaming/error)
type OpenAIErrorResponse struct {
	Type           string  `json:"type"` // always "error"
	Code           *string `json:"code"`
	Message        string  `json:"message"`
	Param          *string `json:"param"`
	SequenceNumber int     `json:"sequence_number"`
}

// BackendStatus represents information about a running backend
type BackendStatus struct {
	// BackendName is the name of the backend
	BackendName string `json:"backend_name"`
	// ModelName is the name of the model loaded in the backend
	ModelName string `json:"model_name"`
	// Mode is the mode the backend is operating in
	Mode string `json:"mode"`
	// LastUsed represents when this (backend, model, mode) tuple was last used
	LastUsed time.Time `json:"last_used,omitempty"`
	// InUse indicates whether this backend is currently handling a request
	InUse bool `json:"in_use,omitempty"`
}

// DiskUsage represents the disk usage of the models and default backend.
type DiskUsage struct {
	ModelsDiskUsage         int64 `json:"models_disk_usage"`
	DefaultBackendDiskUsage int64 `json:"default_backend_disk_usage"`
}

// UnloadRequest is used to specify which models to unload.
type UnloadRequest struct {
	All     bool     `json:"all"`
	Backend string   `json:"backend"`
	Models  []string `json:"models"`
}

// UnloadResponse is used to return the number of unloaded runners (backend, model).
type UnloadResponse struct {
	UnloadedRunners int `json:"unloaded_runners"`
}

// ConfigureRequest specifies per-model runtime configuration options.
type ConfigureRequest struct {
	Model           string                 `json:"model"`
	Mode            *inference.BackendMode `json:"mode,omitempty"`
	RawRuntimeFlags string                 `json:"raw-runtime-flags,omitempty"`
	inference.BackendConfiguration
}

// ModelConfigEntry represents a model configuration entry with its associated metadata.
type ModelConfigEntry struct {
	Backend string
	Model   string
	ModelID string
	Mode    inference.BackendMode
	Config  inference.BackendConfiguration
}
