package distribution

import (
	"io"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/internal/testutil"
	"github.com/docker/model-runner/pkg/distribution/tarball"
)

func TestLoadModel(t *testing.T) {
	tempDir := t.TempDir()

	// Create client
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Load model
	pr, pw := io.Pipe()
	target, err := tarball.NewTarget(pw)
	if err != nil {
		t.Fatalf("Failed to create target: %v", err)
	}
	done := make(chan error)
	var id string
	go func() {
		var err error
		id, err = client.LoadModel(pr, nil)
		done <- err
	}()
	if err := target.Write(t.Context(), testutil.NewGGUFArtifact(t, testGGUFFile), nil); err != nil {
		t.Fatalf("Failed to write model tarball: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("LoadModel exited with error: %v", err)
	}

	// Ensure model was loaded
	if _, err := client.GetModel(id); err != nil {
		t.Fatalf("Failed to get model: %v", err)
	}
}
