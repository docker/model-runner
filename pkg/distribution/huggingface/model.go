package huggingface

import (
	"context"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"sort"
	"strings"

	"github.com/docker/model-runner/pkg/distribution/builder"
	"github.com/docker/model-runner/pkg/distribution/internal/progress"
	"github.com/docker/model-runner/pkg/distribution/packaging"
	"github.com/docker/model-runner/pkg/distribution/types"
)

// BuildModel downloads files from a HuggingFace repository and constructs an OCI model artifact
// This is the main entry point for pulling native HuggingFace models
func BuildModel(ctx context.Context, client *Client, repo, revision string, tempDir string, progressWriter io.Writer) (types.ModelArtifact, error) {
	// Step 1: List files in the repository
	if progressWriter != nil {
		_ = progress.WriteProgress(progressWriter, "Fetching file list...", 0, 0, 0, "")
	}

	files, err := client.ListFiles(ctx, repo, revision)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}

	// Step 2: Filter to model files (safetensors + configs)
	safetensorsFiles, configFiles := FilterModelFiles(files)

	if len(safetensorsFiles) == 0 {
		return nil, fmt.Errorf("no safetensors files found in repository %s", repo)
	}

	// Combine all files to download
	allFiles := append(safetensorsFiles, configFiles...)

	if progressWriter != nil {
		totalSize := TotalSize(allFiles)
		msg := fmt.Sprintf("Found %d files (%.2f MB total)",
			len(allFiles), float64(totalSize)/1024/1024)
		_ = progress.WriteProgress(progressWriter, msg, uint64(totalSize), 0, 0, "")
	}

	// Step 3: Download all files
	downloader := NewDownloader(client, repo, revision, tempDir)
	result, err := downloader.DownloadAll(ctx, allFiles, progressWriter)
	if err != nil {
		return nil, fmt.Errorf("download files: %w", err)
	}

	// Step 4: Build the model artifact
	if progressWriter != nil {
		_ = progress.WriteProgress(progressWriter, "Building model artifact...", 0, 0, 0, "")
	}

	model, err := buildModelFromFiles(result.LocalPaths, safetensorsFiles, configFiles, tempDir)
	if err != nil {
		return nil, fmt.Errorf("build model: %w", err)
	}

	return model, nil
}

// buildModelFromFiles constructs an OCI model artifact from downloaded files
func buildModelFromFiles(localPaths map[string]string, safetensorsFiles, configFiles []RepoFile, tempDir string) (types.ModelArtifact, error) {
	// Collect safetensors paths (sorted for reproducibility)
	var safetensorsPaths []string
	for _, f := range safetensorsFiles {
		localPath, ok := localPaths[f.Path]
		if !ok {
			return nil, fmt.Errorf("missing local path for %s", f.Path)
		}
		safetensorsPaths = append(safetensorsPaths, localPath)
	}
	sort.Strings(safetensorsPaths)

	// Create builder from safetensors files
	b, err := builder.FromSafetensors(safetensorsPaths)
	if err != nil {
		return nil, fmt.Errorf("create builder: %w", err)
	}

	// Create config archive if we have config files
	if len(configFiles) > 0 {
		configArchive, configArchiveErr := createConfigArchive(localPaths, configFiles, tempDir)
		if configArchiveErr != nil {
			return nil, fmt.Errorf("create config archive: %w", configArchiveErr)
		}
		// Note: configArchive is created inside tempDir and will be cleaned up when
		// the caller removes tempDir. The file must exist until after store.Write()
		// completes since the model artifact references it lazily.

		if configArchive != "" {
			var withConfigErr error
			b, withConfigErr = b.WithConfigArchive(configArchive)
			if withConfigErr != nil {
				return nil, fmt.Errorf("add config archive: %w", withConfigErr)
			}
		}
	}

	// Check for chat template and add it
	for _, f := range configFiles {
		if isChatTemplate(f.Path) {
			localPath := localPaths[f.Path]
			b, err = b.WithChatTemplateFile(localPath)
			if err != nil {
				// Non-fatal: log warning but continue to try other potential templates
				log.Printf("Warning: failed to add chat template from %s: %v", f.Path, err)
				continue
			}
			break // Only add one chat template
		}
	}

	return b.Model(), nil
}

// createConfigArchive creates a tar archive of config files in the specified tempDir
func createConfigArchive(localPaths map[string]string, configFiles []RepoFile, tempDir string) (string, error) {
	// Collect config file paths (excluding chat templates which are added separately)
	var configPaths []string
	for _, f := range configFiles {
		if isChatTemplate(f.Path) {
			continue // Chat templates are added as separate layers
		}
		localPath, ok := localPaths[f.Path]
		if !ok {
			return "", fmt.Errorf("internal error: missing local path for downloaded config file %s", f.Path)
		}
		configPaths = append(configPaths, localPath)
	}

	if len(configPaths) == 0 {
		// No config files to archive
		return "", nil
	}

	// Sort for reproducibility
	sort.Strings(configPaths)

	// Create the archive in our tempDir so it gets cleaned up with everything else
	archivePath, err := packaging.CreateConfigArchiveInDir(configPaths, tempDir)
	if err != nil {
		return "", fmt.Errorf("create config archive: %w", err)
	}

	return archivePath, nil
}

// isChatTemplate checks if a file is a chat template
func isChatTemplate(path string) bool {
	filename := filepath.Base(path)
	lower := strings.ToLower(filename)
	return strings.HasSuffix(lower, ".jinja") ||
		strings.Contains(lower, "chat_template") ||
		filename == "tokenizer_config.json" // Often contains chat_template
}
