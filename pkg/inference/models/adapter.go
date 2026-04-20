package models

import (
	"fmt"

	"github.com/docker/model-runner/pkg/distribution/modelpack"
	"github.com/docker/model-runner/pkg/distribution/types"
)

// normalizeConfig converts a ModelPack config to Docker format types.Config
// so that the API wire format is always consistent. This ensures clients
// don't need to understand both config formats.
func normalizeConfig(cfg types.ModelConfig) types.ModelConfig {
	if cfg == nil {
		return nil
	}
	if _, ok := cfg.(*modelpack.Model); ok {
		return &types.Config{
			Format:       cfg.GetFormat(),
			Parameters:   cfg.GetParameters(),
			Quantization: cfg.GetQuantization(),
			Architecture: cfg.GetArchitecture(),
			Size:         cfg.GetSize(),
		}
	}
	return cfg
}

func ToModel(m types.Model) (*Model, error) {
	desc, err := m.Descriptor()
	if err != nil {
		return nil, fmt.Errorf("get descriptor: %w", err)
	}

	id, err := m.ID()
	if err != nil {
		return nil, fmt.Errorf("get id: %w", err)
	}

	cfg, err := m.Config()
	if err != nil {
		return nil, fmt.Errorf("get config: %w", err)
	}

	created := int64(0)
	if desc.Created != nil {
		created = desc.Created.Unix()
	}

	return &Model{
		ID:      id,
		Tags:    m.Tags(),
		Created: created,
		Config:  normalizeConfig(cfg),
	}, nil
}

// ToModelFromArtifact converts a types.ModelArtifact (typically from remote registry)
// to the API Model representation. Remote models don't have tags.
func ToModelFromArtifact(artifact types.ModelArtifact) (*Model, error) {
	desc, err := artifact.Descriptor()
	if err != nil {
		return nil, fmt.Errorf("get descriptor: %w", err)
	}

	id, err := artifact.ID()
	if err != nil {
		return nil, fmt.Errorf("get id: %w", err)
	}

	cfg, err := artifact.Config()
	if err != nil {
		return nil, fmt.Errorf("get config: %w", err)
	}

	created := int64(0)
	if desc.Created != nil {
		created = desc.Created.Unix()
	}

	return &Model{
		ID:      id,
		Tags:    nil, // Remote models don't have local tags
		Created: created,
		Config:  normalizeConfig(cfg),
	}, nil
}
