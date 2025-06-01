package scheduling

import (
	"strings"
	"time"

	"github.com/docker/model-runner/pkg/inference"
)

const (
	// maximumOpenAIInferenceRequestSize defines the maximum size (in bytes) 
	// allowed for an OpenAI API embedding or completion request.
	// It should be large enough for real-world usage but small enough 
	// to mitigate DoS risks.
	maximumOpenAIInferenceRequestSize = 10 * 1024 * 1024 // 10 MB
)

// trimRequestPathToOpenAIRoot returns the substring of path starting from
// the first occurrence of "/v1/". If not found, it returns the original path.
func trimRequestPathToOpenAIRoot(path string) string {
	if idx := strings.Index(path, "/v1/"); idx != -1 {
		return path[idx:]
	}
	return path
}

// backendModeForRequest maps an OpenAI API path to the appropriate
// inference backend mode. Returns the mode and true if a valid mode is determined,
// otherwise returns false.
func backendModeForRequest(path string) (inference.BackendMode, bool) {
	switch {
	case strings.HasSuffix(path, "/v1/chat/completions"), strings.HasSuffix(path, "/v1/completions"):
		return inference.BackendModeCompletion, true
	case strings.HasSuffix(path, "/v1/embeddings"):
		return inference.BackendModeEmbedding, true
	default:
		return inference.BackendMode(0), false
	}
}

// OpenAIInferenceRequest represents the model information extracted from
// a chat completion or embedding request payload to the OpenAI API.
type OpenAIInferenceRequest struct {
	// Model specifies the model name requested.
	Model string `json:"model"`
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
