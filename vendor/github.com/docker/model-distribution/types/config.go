package types

import (
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

const (
	// modelConfigPrefix is the prefix for all versioned model config media types.
	modelConfigPrefix = "application/vnd.docker.ai.model.config"

	// MediaTypeModelConfigV01 is the media type for the model config json.
	MediaTypeModelConfigV01 = types.MediaType("application/vnd.docker.ai.model.config.v0.1+json")

	// MediaTypeGGUF indicates a file in GGUF version 3 format, containing a tensor model.
	MediaTypeGGUF = types.MediaType("application/vnd.docker.ai.gguf.v3")

	// MediaTypeLicense indicates a plain text file containing a license
	MediaTypeLicense = types.MediaType("application/vnd.docker.ai.license")

	// MediaTypeMultimodalProjector indicates a Multimodal projector file
	MediaTypeMultimodalProjector = types.MediaType("application/vnd.docker.ai.mmproj")

	FormatGGUF = Format("gguf")
)

type Format string

type ConfigFile struct {
	Config     Config     `json:"config"`
	Descriptor Descriptor `json:"descriptor"`
	RootFS     v1.RootFS  `json:"rootfs"`
}

// Config describes the model.
type Config struct {
	Format       Format            `json:"format,omitempty"`
	Quantization string            `json:"quantization,omitempty"`
	Parameters   string            `json:"parameters,omitempty"`
	Architecture string            `json:"architecture,omitempty"`
	Size         string            `json:"size,omitempty"`
	GGUF         map[string]string `json:"gguf,omitempty"`
	ContextSize  *uint64           `json:"context_size,omitempty"`
}

// Descriptor provides metadata about the provenance of the model.
type Descriptor struct {
	Created *time.Time `json:"created,omitempty"`
}
