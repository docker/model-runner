package types

import parser "github.com/gpustack/gguf-parser-go"

// ModelDescriptor represents the data of a Model
type ModelDescriptor interface {
	// GetParameters returns the model parameters (raw count, formatted string, error)
	GetParameters() (float64, string, error)

	// GetArchitecture returns the model architecture
	GetArchitecture() string

	// GetQuantization returns the model quantization
	GetQuantization() parser.GGUFFileType

	// GetSize returns the model size (bytes, error)
	GetSize() (uint64, error)

	// GetContextLength returns the model context length (context length, error)
	GetContextLength() (uint32, error)

	// GetVRAM returns the estimated VRAM requirements (bytes, error)
	GetVRAM() (float64, error)

	// GetMetadata returns the model metadata (map[string]string)
	GetMetadata() map[string]string
}
