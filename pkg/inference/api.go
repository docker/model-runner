package inference

// ExperimentalEndpointsPrefix is used to prefix all <paths.InferencePrefix> routes on the Docker
// socket while they are still in their experimental stage. This prefix doesn't
// apply to endpoints on model-runner.docker.internal.
const ExperimentalEndpointsPrefix = "/exp/vDD4.40"

// InferencePrefix is the prefix for inference related routes.
var InferencePrefix = "/engines"

// ModelsPrefix is the prefix for all model manager related routes.
var ModelsPrefix = "/models"

// RequestOriginHeader is the HTTP header used to track the origin of inference requests.
// This header is set internally by proxy handlers (e.g., Ollama compatibility layer)
// to provide more granular tracking of model usage by source.
const RequestOriginHeader = "X-Request-Origin"

// Valid origin values for the RequestOriginHeader.
const (
	// OriginOllamaCompletion indicates the request came from the Ollama /api/chat or /api/generate endpoints
	OriginOllamaCompletion = "ollama/completion"
	// OriginAnthropicMessages indicates the request came from the Anthropic /v1/messages endpoint
	OriginAnthropicMessages = "anthropic/messages"
)
