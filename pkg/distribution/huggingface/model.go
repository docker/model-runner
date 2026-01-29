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
// The tag parameter is used for GGUF repos to select the requested quantization (e.g., "Q4_K_M")
func BuildModel(ctx context.Context, client *Client, repo, revision, tag string, tempDir string, progressWriter io.Writer) (types.ModelArtifact, error) {
	// List files in the repository
	if progressWriter != nil {
		_ = progress.WriteProgress(progressWriter, "Fetching file list...", 0, 0, 0, "", "pull")
	}

	files, err := client.ListFiles(ctx, repo, revision)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}

	// Filter to model files (weights + configs)
	weightFiles, configFiles := FilterModelFiles(files)

	if len(weightFiles) == 0 {
		return nil, fmt.Errorf("no model weight files (GGUF or SafeTensors) found in repository %s", repo)
	}

	// For GGUF repos with multiple quantizations, select the appropriate files
	var mmprojFile *RepoFile
	if isGGUFModel(weightFiles) && len(weightFiles) > 1 {
		// Use the tag as quantization hint (e.g., "Q4_K_M", "Q8_0", or "latest")
		weightFiles, mmprojFile = SelectGGUFFiles(weightFiles, tag)
		if len(weightFiles) == 0 {
			return nil, fmt.Errorf("no GGUF files found matching quantization %q in repository %s", tag, repo)
		}

		if progressWriter != nil {
			if tag == "" || tag == "latest" || tag == "main" {
				_ = progress.WriteProgress(progressWriter, fmt.Sprintf("Selected %s quantization (default)", DefaultGGUFQuantization), 0, 0, 0, "", "pull")
			} else {
				_ = progress.WriteProgress(progressWriter, fmt.Sprintf("Selected %s quantization", tag), 0, 0, 0, "", "pull")
			}
		}
	}

	// Combine all files to download
	allFiles := append(weightFiles, configFiles...)
	if mmprojFile != nil {
		allFiles = append(allFiles, *mmprojFile)
	}

	if progressWriter != nil {
		totalSize := TotalSize(allFiles)
		msg := fmt.Sprintf("Found %d files (%.2f MB total)",
			len(allFiles), float64(totalSize)/1024/1024)
		_ = progress.WriteProgress(progressWriter, msg, uint64(totalSize), 0, 0, "", "pull")
	}

	// Step 3: Download all files
	downloader := NewDownloader(client, repo, revision, tempDir)
	result, err := downloader.DownloadAll(ctx, allFiles, progressWriter)
	if err != nil {
		return nil, fmt.Errorf("download files: %w", err)
	}

	// Step 4: Build the model artifact
	if progressWriter != nil {
		_ = progress.WriteProgress(progressWriter, "Building model artifact...", 0, 0, 0, "", "pull")
	}

	model, err := buildModelFromFiles(result.LocalPaths, weightFiles, configFiles, tempDir)
	if err != nil {
		return nil, fmt.Errorf("build model: %w", err)
	}

	return model, nil
}

// buildModelFromFiles constructs an OCI model artifact from downloaded files
func buildModelFromFiles(localPaths map[string]string, weightFiles, configFiles []RepoFile, tempDir string) (types.ModelArtifact, error) {
	// Collect weight file paths (sorted for reproducibility)
	var weightPaths []string
	for _, f := range weightFiles {
		localPath, ok := localPaths[f.Path]
		if !ok {
			return nil, fmt.Errorf("missing local path for %s", f.Path)
		}
		weightPaths = append(weightPaths, localPath)
	}
	sort.Strings(weightPaths)

	// Create builder from weight files - auto-detects format (GGUF or SafeTensors)
	b, err := builder.FromPaths(weightPaths)
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
