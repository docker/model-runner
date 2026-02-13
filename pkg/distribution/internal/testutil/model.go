package testutil

import (
	"testing"

	"github.com/docker/model-runner/pkg/distribution/builder"
	"github.com/docker/model-runner/pkg/distribution/types"
)

// BuildModelFromPath constructs a model artifact from a file path and fails the test on error.
func BuildModelFromPath(t *testing.T, path string) types.ModelArtifact {
	t.Helper()

	b, err := builder.FromPath(path)
	if err != nil {
		t.Fatalf("Failed to create model from path %q: %v", path, err)
	}
	return b.Model()
}
