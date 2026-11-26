package bundle

import (
	"path/filepath"

	"github.com/docker/model-runner/pkg/distribution/types"
)

const (
	// ModelSubdir is the subdirectory within a bundle where model files are stored
	ModelSubdir = "model"
)

// Bundle represents a runtime bundle containing a model and runtime config
type Bundle struct {
	dir              string
	mmprojPath       string
	ggufFile         string // path to GGUF file (first shard when model is split among files)
	safetensorsFile  string // path to safetensors file (first shard when model is split among files)
	runtimeConfig    types.ModelConfig
	chatTemplatePath string
}

// RootDir return the path to the bundle root directory
func (b *Bundle) RootDir() string {
	return b.dir
}

// GGUFPath return the path to model GGUF file. If the model is sharded this will be the path to the first shard,
// containing metadata headers.
func (b *Bundle) GGUFPath() string {
	if b.ggufFile == "" {
		return ""
	}
	return filepath.Join(b.dir, ModelSubdir, b.ggufFile)
}

// MMPROJPath returns the path to a multi-modal projector file or "" if none is present.
func (b *Bundle) MMPROJPath() string {
	if b.mmprojPath == "" {
		return ""
	}
	return filepath.Join(b.dir, ModelSubdir, b.mmprojPath)
}

// ChatTemplatePath return the path to a Jinja chat template file or "" if none is present.
func (b *Bundle) ChatTemplatePath() string {
	if b.chatTemplatePath == "" {
		return ""
	}
	return filepath.Join(b.dir, ModelSubdir, b.chatTemplatePath)
}

// SafetensorsPath returns the path to model safetensors file. If the model is sharded this will be the path to the first shard.
func (b *Bundle) SafetensorsPath() string {
	if b.safetensorsFile == "" {
		return ""
	}
	return filepath.Join(b.dir, ModelSubdir, b.safetensorsFile)
}

// RuntimeConfig returns config that should be respected by the backend at runtime.
// Can return either Docker format (*types.Config) or ModelPack format (*modelpack.Model).
func (b *Bundle) RuntimeConfig() types.ModelConfig {
	return b.runtimeConfig
}
