package models

import (
	"fmt"

	"github.com/docker/model-runner/pkg/distribution/types"
)

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
		Config:  cfg,
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
		Config:  cfg,
	}, nil
}
