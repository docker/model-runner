// Package modelpack provides native support for CNCF ModelPack format models.
// It enables docker/model-runner to pull, store, and run models in ModelPack format
// without conversion. Both Docker and ModelPack formats are supported natively through
// the types.ModelConfig interface.
//
// Note: JSON tags in this package use camelCase (e.g., "createdAt", "paramSize") to match
// the CNCF ModelPack spec, which differs from Docker model-spec's snake_case convention
// (e.g., "context_size").
//
// See: https://github.com/modelpack/model-spec
package modelpack

import (
	"strings"
	"time"

	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/opencontainers/go-digest"
)

const (
	// MediaTypePrefix is the prefix for all CNCF model config media types.
	MediaTypePrefix = "application/vnd.cncf.model."

	// MediaTypeModelConfigV1 is the CNCF model config v1 media type.
	MediaTypeModelConfigV1 = "application/vnd.cncf.model.config.v1+json"

	// MediaTypeWeightGGUF is the CNCF ModelPack media type for GGUF weight layers.
	MediaTypeWeightGGUF = "application/vnd.cncf.model.weight.v1.gguf"

	// MediaTypeWeightSafetensors is the CNCF ModelPack media type for safetensors weight layers.
	MediaTypeWeightSafetensors = "application/vnd.cncf.model.weight.v1.safetensors"
)

// Model represents the CNCF ModelPack config structure.
// It provides the `application/vnd.cncf.model.config.v1+json` mediatype when marshalled to JSON.
type Model struct {
	// Descriptor provides metadata about the model provenance and identity.
	Descriptor ModelDescriptor `json:"descriptor"`

	// ModelFS describes the layer content addresses.
	ModelFS ModelFS `json:"modelfs"`

	// Config defines the execution parameters for the model.
	Config ModelConfig `json:"config,omitempty"`
}

// ModelDescriptor defines the general information of a model.
type ModelDescriptor struct {
	// CreatedAt is the date and time on which the model was built.
	CreatedAt *time.Time `json:"createdAt,omitempty"`

	// Authors contains the contact details of the people or organization responsible for the model.
	Authors []string `json:"authors,omitempty"`

	// Family is the model family, such as llama3, gpt2, qwen2, etc.
	Family string `json:"family,omitempty"`

	// Name is the model name, such as llama3-8b-instruct, gpt2-xl, etc.
	Name string `json:"name,omitempty"`

	// DocURL is the URL to get documentation on the model.
	DocURL string `json:"docURL,omitempty"`

	// SourceURL is the URL to get source code for building the model.
	SourceURL string `json:"sourceURL,omitempty"`

	// DatasetsURL contains URLs referencing datasets that the model was trained upon.
	DatasetsURL []string `json:"datasetsURL,omitempty"`

	// Version is the version of the packaged software.
	Version string `json:"version,omitempty"`

	// Revision is the source control revision identifier for the packaged software.
	Revision string `json:"revision,omitempty"`

	// Vendor is the name of the distributing entity, organization or individual.
	Vendor string `json:"vendor,omitempty"`

	// Licenses contains the license(s) under which contained software is distributed
	// as an SPDX License Expression.
	Licenses []string `json:"licenses,omitempty"`

	// Title is the human-readable title of the model.
	Title string `json:"title,omitempty"`

	// Description is the human-readable description of the software packaged in the model.
	Description string `json:"description,omitempty"`
}

// ModelConfig defines the execution parameters which should be used as a base
// when running a model using an inference engine.
type ModelConfig struct {
	// Architecture is the model architecture, such as transformer, cnn, rnn, etc.
	Architecture string `json:"architecture,omitempty"`

	// Format is the model format, such as gguf, safetensors, onnx, etc.
	Format string `json:"format,omitempty"`

	// ParamSize is the size of the model parameters, such as "8b", "16b", "32b", etc.
	ParamSize string `json:"paramSize,omitempty"`

	// Precision is the model precision, such as bf16, fp16, int8, mixed etc.
	Precision string `json:"precision,omitempty"`

	// Quantization is the model quantization method, such as awq, gptq, etc.
	Quantization string `json:"quantization,omitempty"`

	// Capabilities defines special capabilities that the model supports.
	Capabilities *ModelCapabilities `json:"capabilities,omitempty"`
}

// ModelCapabilities defines the special capabilities that the model supports.
type ModelCapabilities struct {
	// InputTypes specifies what input modalities the model can process.
	// Values can be: "text", "image", "audio", "video", "embedding", "other".
	InputTypes []string `json:"inputTypes,omitempty"`

	// OutputTypes specifies what output modalities the model can produce.
	// Values can be: "text", "image", "audio", "video", "embedding", "other".
	OutputTypes []string `json:"outputTypes,omitempty"`

	// KnowledgeCutoff is the date of the datasets that the model was trained on.
	KnowledgeCutoff *time.Time `json:"knowledgeCutoff,omitempty"`

	// Reasoning indicates whether the model can perform reasoning tasks.
	Reasoning *bool `json:"reasoning,omitempty"`

	// ToolUsage indicates whether the model can use external tools.
	ToolUsage *bool `json:"toolUsage,omitempty"`

	// Reward indicates whether the model is a reward model.
	Reward *bool `json:"reward,omitempty"`

	// Languages indicates the languages that the model can speak.
	// Encoded as ISO 639 two letter codes. For example, ["en", "fr", "zh"].
	Languages []string `json:"languages,omitempty"`
}

// ModelFS describes the layer content addresses.
type ModelFS struct {
	// Type is the type of the rootfs. MUST be set to "layers".
	Type string `json:"type"`

	// DiffIDs is an array of layer content hashes (DiffIDs),
	// in order from bottom-most to top-most.
	DiffIDs []digest.Digest `json:"diffIds"`
}

// Ensure Model implements types.ModelConfig
var _ types.ModelConfig = (*Model)(nil)

// GetFormat returns the model format, converted to types.Format.
func (m *Model) GetFormat() types.Format {
	f := strings.ToLower(m.Config.Format)
	switch f {
	case "gguf":
		return types.FormatGGUF
	case "safetensors":
		return types.FormatSafetensors
	case "diffusers":
		return types.FormatDiffusers
	default:
		return types.Format(f)
	}
}

// GetContextSize returns the context size. ModelPack spec does not define this field,
// so it always returns nil.
func (m *Model) GetContextSize() *int32 {
	return nil
}

// GetSize returns the parameter size (e.g., "8b").
func (m *Model) GetSize() string {
	return m.Config.ParamSize
}

// GetArchitecture returns the model architecture.
func (m *Model) GetArchitecture() string {
	return m.Config.Architecture
}

// GetParameters returns the parameters description.
// ModelPack uses ParamSize instead of Parameters, so return ParamSize.
func (m *Model) GetParameters() string {
	return m.Config.ParamSize
}

// GetQuantization returns the quantization method.
func (m *Model) GetQuantization() string {
	return m.Config.Quantization
}
