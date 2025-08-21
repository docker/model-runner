package builder

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/model-distribution/internal/gguf"
	"github.com/docker/model-distribution/internal/mutate"
	"github.com/docker/model-distribution/internal/partial"
	"github.com/docker/model-distribution/types"
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

// Target represents a build target
type Target interface {
	Write(context.Context, types.ModelArtifact, io.Writer) error
}

// Build finalizes the artifact and writes it to the given target, reporting progress to the given writer
func (b *Builder) Build(ctx context.Context, target Target, pw io.Writer) error {
	return target.Write(ctx, b.model, pw)
}
