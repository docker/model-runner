package types

import (
	"github.com/docker/model-runner/pkg/distribution/oci"
)

type Model interface {
	ID() (string, error)
	GGUFPaths() ([]string, error)
	SafetensorsPaths() ([]string, error)
	ConfigArchivePath() (string, error)
	MMPROJPath() (string, error)
	Config() (ModelConfig, error)
	Tags() []string
	Descriptor() (Descriptor, error)
	ChatTemplatePath() (string, error)
}

type ModelArtifact interface {
	ID() (string, error)
	Config() (ModelConfig, error)
	Descriptor() (Descriptor, error)
	oci.Image
}

type ModelBundle interface {
	RootDir() string
	GGUFPath() string
	SafetensorsPath() string
	ChatTemplatePath() string
	MMPROJPath() string
	RuntimeConfig() ModelConfig
}
