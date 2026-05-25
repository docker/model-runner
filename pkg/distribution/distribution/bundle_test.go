package distribution

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/internal/testutil"
	"github.com/docker/model-runner/pkg/distribution/types"
)

func TestBundle(t *testing.T) {
	// Create temp directory for store
	tempDir := t.TempDir()

	// Create client
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	mdl := testutil.NewGGUFArtifact(t, filepath.Join("..", "assets", "dummy.gguf"))
	singleGGUFID, err := mdl.ID()
	if err != nil {
		t.Fatalf("Failed to get model ID: %v", err)
	}
	if err := client.store.Write(mdl, []string{"some-model"}, nil); err != nil {
		t.Fatalf("Failed to write model to store: %v", err)
	}

	mmprojMdl := testutil.NewGGUFArtifact(
		t,
		filepath.Join("..", "assets", "dummy.gguf"),
		testutil.Layer(filepath.Join("..", "assets", "dummy.mmproj"), types.MediaTypeMultimodalProjector),
	)
	mmprojMdlID, err := mmprojMdl.ID()
	if err != nil {
		t.Fatalf("Failed to get model ID: %v", err)
	}
	if err := client.store.Write(mmprojMdl, []string{"some-sharded-model"}, nil); err != nil {
		t.Fatalf("Failed to write model to store: %v", err)
	}

	templateMdl := testutil.NewGGUFArtifact(
		t,
		filepath.Join("..", "assets", "dummy.gguf"),
		testutil.Layer(filepath.Join("..", "assets", "template.jinja"), types.MediaTypeChatTemplate),
	)
	templateMdlID, err := templateMdl.ID()
	if err != nil {
		t.Fatalf("Failed to get model ID: %v", err)
	}
	if err := client.store.Write(templateMdl, []string{"some-model-with-template"}, nil); err != nil {
		t.Fatalf("Failed to write model to store: %v", err)
	}

	shardedMdl := testutil.NewDockerArtifact(
		t,
		types.Config{Format: types.FormatGGUF},
		testutil.Layer(filepath.Join("..", "assets", "dummy-00001-of-00002.gguf"), types.MediaTypeGGUF),
		testutil.Layer(filepath.Join("..", "assets", "dummy-00002-of-00002.gguf"), types.MediaTypeGGUF),
	)
	shardedGGUFID, err := shardedMdl.ID()
	if err != nil {
		t.Fatalf("Failed to get model ID: %v", err)
	}
	if err := client.store.Write(shardedMdl, []string{"some-sharded-model"}, nil); err != nil {
		t.Fatalf("Failed to write model to store: %v", err)
	}

	type testCase struct {
		ref           string
		expectedFiles map[string]string //
		description   string
		expectedErr   error
	}

	tcs := []testCase{
		{
			ref:         "not-existing:tag",
			expectedErr: ErrModelNotFound,
			description: "no such model",
		},
		{
			ref:         singleGGUFID,
			description: "single file GGUF by ID",
			expectedFiles: map[string]string{
				"model/model.gguf": filepath.Join("..", "assets", "dummy.gguf"),
			},
		},
		{
			ref:         shardedGGUFID,
			description: "sharded GGUF by ID",
			expectedFiles: map[string]string{
				"model/model-00001-of-00002.gguf": filepath.Join("..", "assets", "dummy-00001-of-00002.gguf"),
				"model/model-00002-of-00002.gguf": filepath.Join("..", "assets", "dummy-00002-of-00002.gguf"),
			},
		},
		{
			ref:         mmprojMdlID,
			description: "model with mmproj file",
			expectedFiles: map[string]string{
				"model/model.gguf":   filepath.Join("..", "assets", "dummy.gguf"),
				"model/model.mmproj": filepath.Join("..", "assets", "dummy.mmproj"),
			},
		},
		{
			ref:         templateMdlID,
			description: "model with template file",
			expectedFiles: map[string]string{
				"model/model.gguf":     filepath.Join("..", "assets", "dummy.gguf"),
				"model/template.jinja": filepath.Join("..", "assets", "template.jinja"),
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.description, func(t *testing.T) {
			bundle, err := client.GetBundle(tc.ref)
			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("Expected error %v, got: %v", tc.expectedErr, err)
			}
			if tc.expectedErr != nil {
				return
			}
			for expectedName, shouldMatchContent := range tc.expectedFiles {
				got, err := os.ReadFile(filepath.Join(bundle.RootDir(), expectedName))
				if err != nil {
					t.Fatalf("Failed to read file: %v", err)
				}
				expected, err := os.ReadFile(shouldMatchContent)
				if err != nil {
					t.Fatalf("Failed to read file with expected contents: %v", err)
				}
				if string(got) != string(expected) {
					t.Fatalf("File contents did not match expected contents. Expected: %s, got: %s", expected, got)
				}
			}
		})
	}
}
