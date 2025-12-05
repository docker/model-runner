package types

import (
	"time"

	v1 "github.com/docker/model-runner/pkg/go-containerregistry/pkg/v1"
	"github.com/docker/model-runner/pkg/go-containerregistry/pkg/v1/types"
)

const (
	// MediaTypeModelConfigV01 is the media type for the model config json.
	MediaTypeModelConfigV01 = types.MediaType("application/vnd.docker.ai.model.config.v0.1+json")

	// MediaTypeGGUF indicates a file in GGUF version 3 format, containing a tensor model.
	MediaTypeGGUF = types.MediaType("application/vnd.docker.ai.gguf.v3")

	// MediaTypeSafetensors indicates a file in safetensors format, containing model weights.
	MediaTypeSafetensors = types.MediaType("application/vnd.docker.ai.safetensors")

	// MediaTypeVLLMConfigArchive indicates a tar archive containing vLLM-specific config files.
	MediaTypeVLLMConfigArchive = types.MediaType("application/vnd.docker.ai.vllm.config.tar")

	// MediaTypeDirTar indicates a tar archive containing a directory with its structure preserved.
	MediaTypeDirTar = types.MediaType("application/vnd.docker.ai.dir.tar")

	// MediaTypeLicense indicates a plain text file containing a license
	MediaTypeLicense = types.MediaType("application/vnd.docker.ai.license")

	// MediaTypeMultimodalProjector indicates a Multimodal projector file
	MediaTypeMultimodalProjector = types.MediaType("application/vnd.docker.ai.mmproj")

	// MediaTypeChatTemplate indicates a Jinja chat template
	MediaTypeChatTemplate = types.MediaType("application/vnd.docker.ai.chat.template.jinja")

	FormatGGUF        = Format("gguf")
	FormatSafetensors = Format("safetensors")

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
	Safetensors  map[string]string `json:"safetensors,omitempty"`
	ContextSize  *int32            `json:"context_size,omitempty"`
}

// Descriptor provides metadata about the provenance of the model.
type Descriptor struct {
	Created *time.Time `json:"created,omitempty"`
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
