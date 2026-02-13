package huggingface

import (
	"context"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/docker/model-runner/pkg/distribution/builder"
	"github.com/docker/model-runner/pkg/distribution/internal/progress"
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

	// Step 4: Fetch repo metadata to get a deterministic creation timestamp.
	// Using the HuggingFace lastModified date ensures the same revision always
	// produces the same OCI digest regardless of when it was pulled.
	var createdTime *time.Time
	repoInfo, err := client.GetRepoInfo(ctx, repo, revision)
	if err != nil {
		log.Printf("Warning: could not fetch repo info for deterministic timestamp: %v. Falling back to current time.", err)
	} else if !repoInfo.LastModified.IsZero() {
		createdTime = &repoInfo.LastModified
	}

	// Step 5: Build the model artifact
	if progressWriter != nil {
		_ = progress.WriteProgress(progressWriter, "Building model artifact...", 0, 0, 0, "", "pull")
	}

	model, err := buildModelFromFiles(result.LocalPaths, weightFiles, configFiles, tempDir, createdTime)
	if err != nil {
		return nil, fmt.Errorf("build model: %w", err)
	}

	return model, nil
}

// buildModelFromFiles constructs an OCI model artifact from downloaded files.
// For safetensors models, it uses the V0.2 layer-per-file packaging (FromDirectory)
// which preserves directory structure and adds each file as an individual layer with
// filepath annotations. For GGUF models, it uses the V0.1 packaging (FromPaths)
// for backward compatibility.
func buildModelFromFiles(localPaths map[string]string, weightFiles, configFiles []RepoFile, tempDir string, createdTime *time.Time) (types.ModelArtifact, error) {
	// Check if this is a safetensors model - use V0.2 packaging
	if isSafetensorsModel(weightFiles) {
		return buildSafetensorsModelV02(tempDir, createdTime)
	}

	// For GGUF models, use V0.1 packaging (backward compatible)
	return buildGGUFModelV01(localPaths, weightFiles, configFiles, createdTime)
}

// buildSafetensorsModelV02 builds a safetensors model using V0.2 layer-per-file packaging.
// It uses builder.FromDirectory which recursively scans the tempDir and creates one layer
// per file, preserving nested directory structure with filepath annotations.
// If createdTime is non-nil, it is used as the creation timestamp for the OCI config
// to produce deterministic digests. Otherwise time.Now() is used.
func buildSafetensorsModelV02(tempDir string, createdTime *time.Time) (types.ModelArtifact, error) {
	var dirOpts []builder.DirectoryOption
	if createdTime != nil {
		dirOpts = append(dirOpts, builder.WithCreatedTime(*createdTime))
	}

	b, err := builder.FromDirectory(tempDir, dirOpts...)
	if err != nil {
		return nil, fmt.Errorf("create builder from directory: %w", err)
	}

	return b.Model(), nil
}

// buildGGUFModelV01 builds a GGUF model using V0.1 packaging (backward compatible).
// GGUF uses FromPaths + config archive approach.
func buildGGUFModelV01(localPaths map[string]string, weightFiles, configFiles []RepoFile, createdTime *time.Time) (types.ModelArtifact, error) {
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

	// Build options for deterministic timestamps
	var buildOpts []builder.BuildOption
	if createdTime != nil {
		buildOpts = append(buildOpts, builder.WithCreated(*createdTime))
	}

	// Create builder from weight files - auto-detects format (GGUF or SafeTensors)
	b, err := builder.FromPaths(weightPaths, buildOpts...)
	if err != nil {
		return nil, fmt.Errorf("create builder: %w", err)
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

// isChatTemplate checks if a file is a chat template
func isChatTemplate(path string) bool {
	filename := filepath.Base(path)
	lower := strings.ToLower(filename)
	return strings.HasSuffix(lower, ".jinja") ||
		strings.Contains(lower, "chat_template") ||
		filename == "tokenizer_config.json" // Often contains chat_template
}
