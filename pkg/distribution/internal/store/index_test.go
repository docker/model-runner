package store_test

import (
	"testing"

	"github.com/docker/model-runner/pkg/distribution/internal/store"
)

func TestMatchReference(t *testing.T) {
	type testCase struct {
		entry       store.IndexEntry
		reference   string
		shouldMatch bool
		description string
	}
	tcs := []testCase{
		{
			entry: store.IndexEntry{
				ID:   "sha256:232a0650cd323d3b760854c4030f63ef11023d6eb3ef78327883f3f739f99def",
				Tags: []string{"some-repo:latest", "some-repo:some-tag"},
			},
			reference:   "sha256:232a0650cd323d3b760854c4030f63ef11023d6eb3ef78327883f3f739f99def",
			shouldMatch: true,
			description: "ID match",
		},
		{
			entry: store.IndexEntry{
				ID:   "sha256:232a0650cd323d3b760854c4030f63ef11023d6eb3ef78327883f3f739f99def",
				Tags: []string{"some-repo:latest", "some-repo:some-tag"},
			},
			reference:   "some-repo:some-tag",
			shouldMatch: true,
			description: "exact tag match",
		},
		{
			entry: store.IndexEntry{
				ID:   "sha256:232a0650cd323d3b760854c4030f63ef11023d6eb3ef78327883f3f739f99def",
				Tags: []string{"some-repo:latest", "some-repo:some-tag"},
			},
			reference:   "some-repo",
			shouldMatch: true,
			description: "implicit tag match",
		},
		{
			entry: store.IndexEntry{
				ID:   "sha256:232a0650cd323d3b760854c4030f63ef11023d6eb3ef78327883f3f739f99def",
				Tags: []string{"some-repo:latest", "some-repo:some-tag"},
			},
			reference:   "docker.io/ai/some-repo:latest",
			shouldMatch: true,
			description: "implicit registry match",
		},
		{
			entry: store.IndexEntry{
				ID:   "sha256:232a0650cd323d3b760854c4030f63ef11023d6eb3ef78327883f3f739f99def",
				Tags: []string{"some-repo:latest", "some-repo:some-tag"},
			},
			reference:   "docker.io/some-org/some-repo:some-tag",
			shouldMatch: false,
			description: "mismatch tag reference",
		},
		{
			entry: store.IndexEntry{
				ID:   "sha256:232a0650cd323d3b760854c4030f63ef11023d6eb3ef78327883f3f739f99def",
				Tags: []string{"some-repo:latest", "some-repo:some-tag"},
			},
			reference:   "docker.io/some-org/some-repo@sha256:232a0650cd323d3b760854c4030f63ef11023d6eb3ef78327883f3f739f99def",
			shouldMatch: true,
			description: "digest reference match",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.description, func(t *testing.T) {
			if tc.entry.MatchesReference(tc.reference) != tc.shouldMatch {
				t.Errorf("Expected %v, got %v", tc.shouldMatch, !tc.shouldMatch)
			}
		})
	}
}

func TestTag(t *testing.T) {
	t.Run("Tagging an entry", func(t *testing.T) {
		// Use normalized tag format since reference package normalizes all tags
		// The default org is "ai", so tags are normalized to docker.io/ai/...
		idx := store.Index{
			Models: []store.IndexEntry{
				{
					ID:   "some-id",
					Tags: []string{"docker.io/ai/some-tag:latest"},
				},
				{
					ID:   "other-id",
					Tags: []string{"docker.io/ai/other-tag:latest"},
				},
			},
		}
		idx, err := idx.Tag("some-id", "other-tag")
		if err != nil {
			t.Fatalf("Error tagging entry: %v", err)
		}
		// Check that both models are still present
		if len(idx.Models) != 2 {
			t.Fatalf("Expected 2 models, got %d", len(idx.Models))
		}
		if idx.Models[0].ID != "some-id" {
			t.Fatalf("Expected ID 'some-id', got '%s'", idx.Models[0].ID)
		}
		if idx.Models[1].ID != "other-id" {
			t.Fatalf("Expected ID 'other-id', got '%s'", idx.Models[1].ID)
		}

		// Check that new tag is added to the first model
		if len(idx.Models[0].Tags) != 2 {
			t.Fatalf("Expected 2 tags, got %d", len(idx.Models[0].Tags))
		}
		// Tags are normalized to full docker.io/ai form (default org is "ai")
		if idx.Models[0].Tags[1] != "docker.io/ai/other-tag:latest" {
			t.Fatalf("Expected tag 'docker.io/ai/other-tag:latest', got '%s'", idx.Models[0].Tags[1])
		}

		// Check that tag is removed from the second model
		if len(idx.Models[1].Tags) != 0 {
			t.Fatalf("Expected 0 tags, got %d", len(idx.Models[1].Tags))
		}

		// Try to add a redundant tag
		idx, err = idx.Tag("some-id", "other-tag")
		if err != nil {
			t.Fatalf("Error tagging entry: %v", err)
		}
		// Check that the tag was not added again
		if len(idx.Models[0].Tags) != 2 {
			t.Fatalf("Expected 2 tags, got %d", len(idx.Models[0].Tags))
		}
	})
}

func TestUntag(t *testing.T) {
	t.Run("UnTagging an entry", func(t *testing.T) {
		// Use normalized tag format since reference package normalizes all tags
		// The default org is "ai", so tags are normalized to docker.io/ai/...
		idx := store.Index{
			Models: []store.IndexEntry{
				{
					ID:   "some-id",
					Tags: []string{"docker.io/ai/some-tag:latest", "docker.io/ai/other-tag:latest"},
				},
				{
					ID:   "other-id",
					Tags: []string{},
				},
			},
		}
		t.Run("UnTagging existing tag", func(t *testing.T) {
			tag, newIdx, err := idx.UnTag("other-tag")
			if err != nil {
				t.Fatalf("Error untagging entry: %v", err)
			}
			if len(newIdx.Models) != 2 {
				t.Fatalf("Expected 2 models, got %d", len(newIdx.Models))
			}
			if len(newIdx.Models[0].Tags) != 1 {
				t.Fatalf("Expected 1 tag, got %d", len(newIdx.Models[0].Tags))
			}
			// Tags are normalized to full docker.io/ai form (default org is "ai")
			if tag.String() != "docker.io/ai/other-tag:latest" {
				t.Fatalf("Expected tag 'docker.io/ai/other-tag:latest', got '%s'", tag)
			}
		})
		t.Run("UnTagging invalid tag", func(t *testing.T) {
			_, _, err := idx.UnTag("!@#$%^&*()")
			if err == nil {
				t.Fatal("Expected error when untagging non-existing tag, got nil")
			}
		})
	})
}
