package search

import (
	"context"
	"errors"
	"strings"

	"github.com/docker/model-runner/pkg/distribution/files"
	distributionhf "github.com/docker/model-runner/pkg/distribution/huggingface"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/registry"
	disttypes "github.com/docker/model-runner/pkg/distribution/types"
	"golang.org/x/sync/errgroup"
)

const (
	backendUnknown = "unknown"

	backendLlamaCpp  = "llama.cpp"
	backendVLLM      = "vllm"
	backendDiffusers = "diffusers"

	defaultBackendResolveConcurrency = 4
)

type backendResolver interface {
	Resolve(ctx context.Context, target string) (backend string, size int64, err error)
}

type registryBackendResolver struct {
	lookup func(ctx context.Context, reference string) (disttypes.ModelArtifact, error)
}

func newRegistryBackendResolver() *registryBackendResolver {
	client := registry.NewClient()
	return &registryBackendResolver{
		lookup: client.Model,
	}
}

func (r *registryBackendResolver) Resolve(ctx context.Context, target string) (string, int64, error) {
	model, err := r.lookup(ctx, withDefaultTag(target))
	if err != nil {
		return backendUnknown, 0, err
	}

	backend := backendUnknown
	config, configErr := model.Config()
	if configErr == nil {
		backend = backendFromFormat(config.GetFormat())
	}

	manifest, manifestErr := model.Manifest()
	if manifestErr != nil {
		if configErr != nil {
			return backendUnknown, 0, errors.Join(configErr, manifestErr)
		}
		return backend, 0, manifestErr
	}

	if backend == backendUnknown {
		backend = backendFromManifestLayers(manifest)
	}

	var totalSize int64
	if manifest != nil {
		for _, layer := range manifest.Layers {
			totalSize += layer.Size
		}
	}

	if backend == backendUnknown && configErr != nil {
		return backendUnknown, totalSize, configErr
	}

	return backend, totalSize, nil
}

type huggingFaceRepoBackendResolver struct {
	listFiles func(ctx context.Context, repo, revision string) ([]distributionhf.RepoFile, error)
}

func newHuggingFaceRepoBackendResolver() *huggingFaceRepoBackendResolver {
	client := distributionhf.NewClient(distributionhf.WithUserAgent(registry.DefaultUserAgent))
	return &huggingFaceRepoBackendResolver{
		listFiles: client.ListFiles,
	}
}

func (r *huggingFaceRepoBackendResolver) Resolve(ctx context.Context, target string) (string, int64, error) {
	repoFiles, err := r.listFiles(ctx, target, "main")
	if err != nil {
		return backendUnknown, 0, err
	}
	return backendFromRepoFiles(repoFiles), distributionhf.TotalSize(repoFiles), nil
}

func backendFromFormat(format disttypes.Format) string {
	switch format {
	case disttypes.FormatGGUF:
		return backendLlamaCpp
	case disttypes.FormatSafetensors:
		return backendVLLM
	case disttypes.FormatDDUF, disttypes.FormatDiffusers: //nolint:staticcheck // FormatDiffusers kept for backward compatibility
		return backendDiffusers
	default:
		return backendUnknown
	}
}

func backendFromManifestLayers(manifest *oci.Manifest) string {
	if manifest == nil {
		return backendUnknown
	}

	var backends []string
	for _, layer := range manifest.Layers {
		//nolint:exhaustive // only backend-relevant media types affect search classification
		switch layer.MediaType {
		case disttypes.MediaTypeGGUF:
			backends = append(backends, backendLlamaCpp)
		case disttypes.MediaTypeSafetensors:
			backends = append(backends, backendVLLM)
		case disttypes.MediaTypeDDUF:
			backends = append(backends, backendDiffusers)
		default:
			continue
		}
	}

	return joinBackends(backends...)
}

func backendFromRepoFiles(repoFiles []distributionhf.RepoFile) string {
	var backends []string
	for _, repoFile := range repoFiles {
		if repoFile.Type != "file" {
			continue
		}

		//nolint:exhaustive // only model weight file types affect search classification
		switch files.Classify(repoFile.Filename()) {
		case files.FileTypeGGUF:
			backends = append(backends, backendLlamaCpp)
		case files.FileTypeSafetensors:
			backends = append(backends, backendVLLM)
		case files.FileTypeDDUF:
			backends = append(backends, backendDiffusers)
		default:
			continue
		}
	}

	return joinBackends(backends...)
}

func resolveSearchResultBackends(
	ctx context.Context,
	results []SearchResult,
	resolveConcurrency int,
	resolve func(context.Context, SearchResult) (string, int64, error),
) []SearchResult {
	if len(results) == 0 {
		return results
	}

	if resolveConcurrency <= 0 {
		resolveConcurrency = defaultBackendResolveConcurrency
	}

	resolved := append([]SearchResult(nil), results...)
	group, workerCtx := errgroup.WithContext(ctx)
	group.SetLimit(resolveConcurrency)

	for i := range resolved {
		group.Go(func() error {
			backend, size, err := resolve(workerCtx, resolved[i])
			if err != nil || backend == "" {
				resolved[i].Backend = backendUnknown
				return nil
			}
			resolved[i].Backend = backend
			resolved[i].Size = size
			return nil
		})
	}

	_ = group.Wait()
	return resolved
}

func joinBackends(backends ...string) string {
	seen := map[string]bool{}
	for _, backend := range backends {
		if backend == "" || backend == backendUnknown {
			continue
		}
		seen[backend] = true
	}

	ordered := []string{
		backendLlamaCpp,
		backendVLLM,
		backendDiffusers,
	}

	var unique []string
	for _, backend := range ordered {
		if seen[backend] {
			unique = append(unique, backend)
		}
	}

	if len(unique) == 0 {
		return backendUnknown
	}

	return strings.Join(unique, ", ")
}

func withDefaultTag(reference string) string {
	lastSlash := strings.LastIndex(reference, "/")
	lastColon := strings.LastIndex(reference, ":")
	lastDigest := strings.LastIndex(reference, "@")

	if lastColon > lastSlash || lastDigest > lastSlash {
		return reference
	}

	return reference + ":latest"
}
