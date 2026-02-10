package domain

import (
	"github.com/docker/model-cards/tools/build-tables/types"
)

// ModelVariant represents a single model variant with its properties
type ModelVariant struct {
	RepoName      string
	Tags          []string
	Architecture  string
	Parameters    string
	Quantization  string
	Size          uint64
	ContextLength uint32
	VRAM          uint64
	Descriptor    types.ModelDescriptor
}

// IsLatest returns true if this variant has the "latest" tag
func (v ModelVariant) IsLatest() bool {
	for _, tag := range v.Tags {
		if tag == "latest" {
			return true
		}
	}
	return false
}

// GetLatestTag returns the non-latest tag that corresponds to the latest tag
func (v ModelVariant) GetLatestTag() string {
	if !v.IsLatest() {
		return ""
	}
	// Return the first non-latest tag
	for _, tag := range v.Tags {
		if tag != "latest" {
			return tag
		}
	}
	return ""
}
