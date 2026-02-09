package types

import (
	"time"

	"github.com/docker/model-runner/pkg/distribution/oci"
)

// MediaType is an alias for oci.MediaType for convenience.
type MediaType = oci.MediaType

const (
	// MediaTypeModelConfigV01 is the media type for the model config json (legacy format).
	// V0.1 uses model.GGUFPaths(), model.SafetensorsPaths(), etc. for unpacking.
	MediaTypeModelConfigV01 MediaType = "application/vnd.docker.ai.model.config.v0.1+json"

	// MediaTypeModelConfigV02 is the media type for models using the layer-per-file approach.
	// V0.2 packages each file as an individual layer with filepath annotations.
	// This format preserves nested directory structure (e.g., text_encoder/model.safetensors).
	// Used by builder.FromDirectory.
	MediaTypeModelConfigV02 MediaType = "application/vnd.docker.ai.model.config.v0.2+json"

	// MediaTypeGGUF indicates a file in GGUF version 3 format, containing a tensor model.
	MediaTypeGGUF MediaType = "application/vnd.docker.ai.gguf.v3"

	// MediaTypeSafetensors indicates a file in safetensors format, containing model weights.
	MediaTypeSafetensors MediaType = "application/vnd.docker.ai.safetensors"

	// MediaTypeVLLMConfigArchive indicates a tar archive containing vLLM-specific config files.
	MediaTypeVLLMConfigArchive MediaType = "application/vnd.docker.ai.vllm.config.tar"

	// MediaTypeDirTar indicates a tar archive containing a directory with its structure preserved.
	MediaTypeDirTar MediaType = "application/vnd.docker.ai.dir.tar"

	// MediaTypeDDUF indicates a file in DDUF format (Diffusers Unified Format).
	MediaTypeDDUF MediaType = "application/vnd.docker.ai.dduf"

	// MediaTypeLicense indicates a plain text file containing a license
	MediaTypeLicense MediaType = "application/vnd.docker.ai.license"

	// MediaTypeMultimodalProjector indicates a Multimodal projector file
	MediaTypeMultimodalProjector MediaType = "application/vnd.docker.ai.mmproj"

	// MediaTypeChatTemplate indicates a Jinja chat template
	MediaTypeChatTemplate MediaType = "application/vnd.docker.ai.chat.template.jinja"

	// MediaTypeModelFile indicates a generic model-related file (config, tokenizer, etc.)
	// The actual file path is stored in the AnnotationFilePath annotation.
	MediaTypeModelFile MediaType = "application/vnd.docker.ai.model.file"

	FormatGGUF        = Format("gguf")
	FormatSafetensors = Format("safetensors")
	FormatDiffusers   = Format("diffusers")

	// OCI Annotation keys for model layers
	// See https://github.com/opencontainers/image-spec/blob/main/annotations.md

	// AnnotationFilePath specifies the file path of the layer (string)
	AnnotationFilePath = "org.cncf.model.filepath"

	// AnnotationFileMetadata specifies the metadata of the file (string), value is the JSON string of FileMetadata
	AnnotationFileMetadata = "org.cncf.model.file.metadata+json"

	// AnnotationMediaTypeUntested indicates whether the media type classification of files in the layer is untested (string)
	// Valid values are "true" or "false". When set to "true", it signals that the model packager has not verified
	// the media type classification and the type is inferred or assumed based on some heuristics.
	AnnotationMediaTypeUntested = "org.cncf.model.file.mediatype.untested"
)

type Format string

// ModelConfig provides a unified interface for accessing model configuration.
// Both Docker format (*Config) and ModelPack format (*modelpack.Model) implement
// this interface, allowing schedulers and backends to access config without
// knowing the underlying format.
type ModelConfig interface {
	GetFormat() Format
	GetContextSize() *int32
	GetSize() string
	GetArchitecture() string
	GetParameters() string
	GetQuantization() string
}

type ConfigFile struct {
	Config     Config     `json:"config"`
	Descriptor Descriptor `json:"descriptor"`
	RootFS     oci.RootFS `json:"rootfs"`
}

// Config describes the model.
type Config struct {
	Format       Format            `json:"format,omitempty"`
	Quantization string            `json:"quantization,omitempty"`
	Parameters   string            `json:"parameters,omitempty"`
	Architecture string            `json:"architecture,omitempty"`
	Size         string            `json:"size,omitempty"`
	GGUF         map[string]string `json:"gguf,omitempty"`
	Safetensors  map[string]string `json:"safetensors,omitempty"`
	Diffusers    map[string]string `json:"diffusers,omitempty"`
	ContextSize  *int32            `json:"context_size,omitempty"`
}

// Descriptor provides metadata about the provenance of the model.
type Descriptor struct {
	Created *time.Time `json:"created,omitempty"`
}

// Ensure Config implements ModelConfig
var _ ModelConfig = (*Config)(nil)

// GetFormat returns the model format.
func (c *Config) GetFormat() Format {
	return c.Format
}

// GetContextSize returns the context size configuration.
func (c *Config) GetContextSize() *int32 {
	return c.ContextSize
}

// GetSize returns the parameter size (e.g., "8B").
func (c *Config) GetSize() string {
	return c.Size
}

// GetArchitecture returns the model architecture.
func (c *Config) GetArchitecture() string {
	return c.Architecture
}

// GetParameters returns the parameters description.
func (c *Config) GetParameters() string {
	return c.Parameters
}

// GetQuantization returns the quantization method.
func (c *Config) GetQuantization() string {
	return c.Quantization
}

// FileMetadata represents the metadata of file, which is the value definition of AnnotationFileMetadata.
// This follows the OCI image specification for model artifacts.
type FileMetadata struct {
	// File name
	Name string `json:"name"`

	// File permission mode (e.g., Unix permission bits)
	Mode uint32 `json:"mode"`

	// User ID (identifier of the file owner)
	Uid uint32 `json:"uid"`

	// Group ID (identifier of the file's group)
	Gid uint32 `json:"gid"`

	// File size (in bytes)
	Size int64 `json:"size"`

	// File last modification time
	ModTime time.Time `json:"mtime"`

	// File type flag (e.g., regular file, directory, etc.)
	Typeflag byte `json:"typeflag"`
}
