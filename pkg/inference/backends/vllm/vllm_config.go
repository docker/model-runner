package vllm

// Config is the configuration for the vLLM backend.
type Config struct {
}

// NewDefaultVLLMConfig creates a new VLLMConfig with default values.
func NewDefaultVLLMConfig() *Config {
	return &Config{}
}
