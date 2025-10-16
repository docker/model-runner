package builder

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/model-runner/pkg/distribution/internal/gguf"
	"github.com/docker/model-runner/pkg/distribution/internal/mutate"
	"github.com/docker/model-runner/pkg/distribution/internal/partial"
	"github.com/docker/model-runner/pkg/distribution/internal/safetensors"
	"github.com/docker/model-runner/pkg/distribution/types"
)

// Builder builds a model artifact
type Builder struct {
	model types.ModelArtifact
}

// FromGGUF returns a *Builder that builds a model artifacts from a GGUF file
func FromGGUF(path string) (*Builder, error) {
	mdl, err := gguf.NewModel(path)
	if err != nil {
		return nil, err
	}
	return &Builder{
		model: mdl,
	}, nil
}

// FromSafetensors returns a *Builder that builds model artifacts from safetensors files
func FromSafetensors(safetensorsPaths []string) (*Builder, error) {
	mdl, err := safetensors.NewModel(safetensorsPaths)
	if err != nil {
		return nil, err
	}
	return &Builder{
		model: mdl,
	}, nil
}

// WithLicense adds a license file to the artifact
func (b *Builder) WithLicense(path string) (*Builder, error) {
	licenseLayer, err := partial.NewLayer(path, types.MediaTypeLicense)
	if err != nil {
		return nil, fmt.Errorf("license layer from %q: %w", path, err)
	}
	return &Builder{
		model: mutate.AppendLayers(b.model, licenseLayer),
	}, nil
}

func (b *Builder) WithContextSize(size uint64) *Builder {
	return &Builder{
		model: mutate.ContextSize(b.model, size),
	}
}

// WithMultimodalProjector adds a Multimodal projector file to the artifact
func (b *Builder) WithMultimodalProjector(path string) (*Builder, error) {
	mmprojLayer, err := partial.NewLayer(path, types.MediaTypeMultimodalProjector)
	if err != nil {
		return nil, fmt.Errorf("mmproj layer from %q: %w", path, err)
	}
	return &Builder{
		model: mutate.AppendLayers(b.model, mmprojLayer),
	}, nil
}

// WithChatTemplateFile adds a Jinja chat template file to the artifact which takes precedence over template from GGUF.
func (b *Builder) WithChatTemplateFile(path string) (*Builder, error) {
	templateLayer, err := partial.NewLayer(path, types.MediaTypeChatTemplate)
	if err != nil {
		return nil, fmt.Errorf("chat template layer from %q: %w", path, err)
	}
	return &Builder{
		model: mutate.AppendLayers(b.model, templateLayer),
	}, nil
}

// WithConfigArchive adds a config archive (tar) file to the artifact
func (b *Builder) WithConfigArchive(path string) (*Builder, error) {
	// Check if config archive already exists
	layers, err := b.model.Layers()
	if err != nil {
		return nil, fmt.Errorf("get model layers: %w", err)
	}

	for _, layer := range layers {
		mediaType, err := layer.MediaType()
		if err == nil && mediaType == types.MediaTypeVLLMConfigArchive {
			return nil, fmt.Errorf("model already has a config archive layer")
		}
	}

	configLayer, err := partial.NewLayer(path, types.MediaTypeVLLMConfigArchive)
	if err != nil {
		return nil, fmt.Errorf("config archive layer from %q: %w", path, err)
	}
	return &Builder{
		model: mutate.AppendLayers(b.model, configLayer),
	}, nil
}

// WithDirTar adds a directory tar archive to the artifact.
// Multiple directory tar archives can be added by calling this method multiple times.
func (b *Builder) WithDirTar(path string) (*Builder, error) {
	dirTarLayer, err := partial.NewLayer(path, types.MediaTypeDirTar)
	if err != nil {
		return nil, fmt.Errorf("dir tar layer from %q: %w", path, err)
	}
	return &Builder{
		model: mutate.AppendLayers(b.model, dirTarLayer),
	}, nil
}

// Target represents a build target
type Target interface {
	Write(context.Context, types.ModelArtifact, io.Writer) error
}

// Build finalizes the artifact and writes it to the given target, reporting progress to the given writer
func (b *Builder) Build(ctx context.Context, target Target, pw io.Writer) error {
	return target.Write(ctx, b.model, pw)
}
