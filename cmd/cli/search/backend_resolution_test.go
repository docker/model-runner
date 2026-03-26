package search

import (
	"context"
	"fmt"
	"testing"

	distributionhf "github.com/docker/model-runner/pkg/distribution/huggingface"
	"github.com/docker/model-runner/pkg/distribution/oci"
	disttypes "github.com/docker/model-runner/pkg/distribution/types"
)

func TestBackendFromFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		format disttypes.Format
		want   string
	}{
		{name: "gguf", format: disttypes.FormatGGUF, want: backendLlamaCpp},
		{name: "safetensors", format: disttypes.FormatSafetensors, want: backendVLLM},
		{name: "dduf", format: disttypes.FormatDDUF, want: backendDiffusers},
		{name: "diffusers (deprecated)", format: disttypes.FormatDiffusers, want: backendDiffusers}, //nolint:staticcheck // FormatDiffusers kept for backward compatibility
		{name: "unknown", format: "", want: backendUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := backendFromFormat(tt.format); got != tt.want {
				t.Fatalf("backendFromFormat(%q) = %q, want %q", tt.format, got, tt.want)
			}
		})
	}
}

func TestBackendFromManifestLayers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		manifest *oci.Manifest
		want     string
	}{
		{
			name: "gguf layer",
			manifest: &oci.Manifest{
				Layers: []oci.Descriptor{{MediaType: disttypes.MediaTypeGGUF}},
			},
			want: backendLlamaCpp,
		},
		{
			name: "dduf layer",
			manifest: &oci.Manifest{
				Layers: []oci.Descriptor{{MediaType: disttypes.MediaTypeDDUF}},
			},
			want: backendDiffusers,
		},
		{
			name: "multiple verified layer types",
			manifest: &oci.Manifest{
				Layers: []oci.Descriptor{
					{MediaType: disttypes.MediaTypeSafetensors},
					{MediaType: disttypes.MediaTypeGGUF},
				},
			},
			want: "llama.cpp, vllm",
		},
		{
			name: "no recognized layers",
			manifest: &oci.Manifest{
				Layers: []oci.Descriptor{{MediaType: disttypes.MediaTypeModelFile}},
			},
			want: backendUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := backendFromManifestLayers(tt.manifest); got != tt.want {
				t.Fatalf("backendFromManifestLayers() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBackendFromRepoFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		repoFiles []distributionhf.RepoFile
		want      string
	}{
		{
			name: "gguf and safetensors repo",
			repoFiles: []distributionhf.RepoFile{
				{Type: "file", Path: "model.gguf"},
				{Type: "file", Path: "weights/model.safetensors"},
			},
			want: "llama.cpp, vllm",
		},
		{
			name: "dduf repo",
			repoFiles: []distributionhf.RepoFile{
				{Type: "file", Path: "sdxl.dduf"},
			},
			want: backendDiffusers,
		},
		{
			name: "no recognized files",
			repoFiles: []distributionhf.RepoFile{
				{Type: "file", Path: "README.md"},
			},
			want: backendUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := backendFromRepoFiles(tt.repoFiles); got != tt.want {
				t.Fatalf("backendFromRepoFiles() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWithDefaultTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		reference string
		want      string
	}{
		{name: "adds latest", reference: "ai/stable-diffusion", want: "ai/stable-diffusion:latest"},
		{name: "keeps existing tag", reference: "ai/stable-diffusion:Q4", want: "ai/stable-diffusion:Q4"},
		{name: "keeps digest", reference: "ai/stable-diffusion@sha256:deadbeef", want: "ai/stable-diffusion@sha256:deadbeef"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := withDefaultTag(tt.reference); got != tt.want {
				t.Fatalf("withDefaultTag(%q) = %q, want %q", tt.reference, got, tt.want)
			}
		})
	}
}

func TestResolveSearchResultBackendsConcurrent(t *testing.T) {
	t.Parallel()

	const numResults = 20

	results := make([]SearchResult, numResults)
	for i := range results {
		results[i] = SearchResult{
			Name:   fmt.Sprintf("model-%d", i),
			Source: "test",
		}
	}

	wantBackends := make([]string, numResults)
	for i := range wantBackends {
		switch i % 3 {
		case 0:
			wantBackends[i] = backendLlamaCpp
		case 1:
			wantBackends[i] = backendVLLM
		case 2:
			wantBackends[i] = backendDiffusers
		}
	}

	resolve := func(_ context.Context, result SearchResult) (string, error) {
		for i, r := range results {
			if r.Name == result.Name {
				return wantBackends[i], nil
			}
		}
		return backendUnknown, nil
	}

	resolved := resolveSearchResultBackends(t.Context(), results, numResults, resolve)

	if len(resolved) != numResults {
		t.Fatalf("expected %d results, got %d", numResults, len(resolved))
	}

	for i, r := range resolved {
		if r.Backend != wantBackends[i] {
			t.Errorf("result[%d] (%s): Backend = %q, want %q", i, r.Name, r.Backend, wantBackends[i])
		}
	}
}
