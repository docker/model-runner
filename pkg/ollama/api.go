package ollama

import "time"

const (
	// APIPrefix Ollama API prefix
	APIPrefix = "/api"
)

// ListResponse is the response for /api/tags
type ListResponse struct {
	Models []ModelResponse `json:"models"`
}

// ModelResponse represents a single model in the list
type ModelResponse struct {
	Name       string       `json:"name"`
	Model      string       `json:"model"`
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

// PSModel represents a running model in the ps response
type PSModel struct {
	Name      string    `json:"name"`
	Model     string    `json:"model"`
	Size      int64     `json:"size"`
	Digest    string    `json:"digest"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	SizeVram  int64     `json:"size_vram,omitempty"`
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
