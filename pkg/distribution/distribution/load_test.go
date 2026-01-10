package distribution

import (
	"io"
	"os"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/builder"
	"github.com/docker/model-runner/pkg/distribution/tarball"
)

func TestLoadModel(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

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
	bldr, err := builder.FromPath(testGGUFFile)
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}
	err = bldr.Build(t.Context(), target, nil)
	if err != nil {
		t.Fatalf("Failed to build model: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("LoadModel exited with error: %v", err)
	}

	// Ensure model was loaded
	if _, err := client.GetModel(id); err != nil {
		t.Fatalf("Failed to get model: %v", err)
	}
}
