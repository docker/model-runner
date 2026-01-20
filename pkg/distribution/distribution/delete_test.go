package distribution

import (
	"errors"
	"os"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/builder"
)

func TestDeleteModel(t *testing.T) {
	tempDir := t.TempDir()

	// Create client
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Use the dummy.gguf file from assets directory
	b, err := builder.FromPath(testGGUFFile)
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}
	mdl := b.Model()
	id, err := mdl.ID()
	if err != nil {
		t.Fatalf("Failed to get model ID: %v", err)
	}
	if err := client.store.Write(mdl, []string{}, nil); err != nil {
		t.Fatalf("Failed to write model to store: %v", err)
	}

	type testCase struct {
		ref         string   // ref to delete by (id or tag)
		tags        []string // applied tags
		force       bool
		expectedErr error
		description string
		untagOnly   bool
	}

	tcs := []testCase{
		{
			ref:         id,
			description: "untagged, by ID",
		},
		{
			ref:         id,
			force:       true,
			description: "untagged, by ID, with force",
		},
		{
			ref:         id,
			tags:        []string{"some-repo:some-tag"},
			description: "one tag, by ID",
		},
		{
			ref:         "some-repo:some-tag",
			tags:        []string{"some-repo:some-tag"},
			description: "one tag, by tag",
		},
		{
			ref:         id,
			tags:        []string{"some-repo:some-tag"},
			force:       true,
			description: "one tag, by ID, with force",
		},
		{
			ref:         "some-repo:some-tag",
			tags:        []string{"some-repo:some-tag"},
			force:       true,
			description: "one tag, by tag, with force",
		},
		{
			ref:         id,
			tags:        []string{"some-repo:some-tag", "other-repo:other-tag"},
			expectedErr: ErrConflict,
			description: "multiple tags, by ID",
		},
		{
			ref:         id,
			tags:        []string{"some-repo:some-tag", "other-repo:other-tag"},
			force:       true,
			description: "multiple tags, by ID, with force",
		},
		{
			ref:         "some-repo:some-tag",
			tags:        []string{"some-repo:some-tag", "other-repo:other-tag"},
			untagOnly:   true,
			description: "multiple tags, by tag",
		},
		{
			ref:         "some-repo:some-tag",
			tags:        []string{"some-repo:some-tag", "other-repo:other-tag"},
			force:       true,
			untagOnly:   true,
			description: "multiple tags, by tag, with force",
		},
		{
			ref:         "not-existing:tag",
			tags:        []string{},
			expectedErr: ErrModelNotFound,
			description: "no such model",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.description, func(t *testing.T) {
			// Setup model with tags and create bundle
			if err := client.store.Write(mdl, []string{}, nil); err != nil {
				t.Fatalf("Failed to write model to store: %v", err)
			}
			for _, tag := range tc.tags {
				if err := client.Tag(id, tag); err != nil {
					t.Fatalf("Failed to tag model: %v", err)
				}
			}
			bundle, err := client.GetBundle(id)
			if err != nil {
				t.Fatalf("Failed to get model bundle: %v", err)
			}

			// Attempt to delete the model and check for expected error
			_, err = client.DeleteModel(tc.ref, tc.force)
			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("Expected error %v, got: %v", tc.expectedErr, err)
			}
			if tc.expectedErr != nil {
				return
			}

			// Verify model ref unreachable by ref (untagged)
			_, err = client.GetModel(tc.ref)
			if !errors.Is(err, ErrModelNotFound) {
				t.Errorf("Expected ErrModelNotFound after deletion, got %v", err)
			}

			// Verify if underlying model is deleted
			if _, err = client.GetModel(id); !tc.untagOnly && !errors.Is(err, ErrModelNotFound) {
				t.Errorf("Expected ErrModelNotFound after deletion, got %v", err)
			} else if tc.untagOnly && err != nil {
				t.Errorf("Expected model to remain but was deleted")
			}

			if _, err := os.Stat(bundle.RootDir()); err != nil && tc.untagOnly {
				t.Fatalf("Expected model bundle dir to remain but was deleted")
			} else if err == nil && !tc.untagOnly {
				t.Fatalf("Expected model bundle dir be deleted")
			}
		})
	}
}
