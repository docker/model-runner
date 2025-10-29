package config

import (
	"github.com/docker/model-runner/pkg/distribution/types"

	"github.com/docker/model-runner/pkg/inference"
)

// BackendConfig is the interface implemented by backend configurations.
// It provides methods to get command-line arguments for a backend based on
// the model path, socket, and mode.
type BackendConfig interface {
	// GetArgs returns the command-line arguments for the backend.
	// It takes the model path, socket, and mode as input and returns
	// the appropriate arguments for the backend.
	GetArgs(bundle types.ModelBundle, socket string, mode inference.BackendMode, config *inference.BackendConfiguration) ([]string, error)
}
