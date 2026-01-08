package huggingface

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"log"
	"os"
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

	// Step 2: Detect model type and filter files accordingly
	modelType := DetectModelType(files)

	var modelFiles, configFiles []RepoFile
	if modelType == ModelTypeDiffusers {
		modelFiles, configFiles = FilterDiffusersFiles(files)
		if progressWriter != nil {
			_ = progress.WriteProgress(progressWriter, "Detected diffusers model", 0, 0, 0, "")
		}
	} else {
		modelFiles, configFiles = FilterModelFiles(files)
	}

	if len(modelFiles) == 0 {
		return nil, fmt.Errorf("no model files found in repository %s", repo)
	}

	// Combine all files to download
	allFiles := append(modelFiles, configFiles...)

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

	var model types.ModelArtifact
	if modelType == ModelTypeDiffusers {
		model, err = buildDiffusersModel(result.LocalPaths, modelFiles, configFiles, tempDir)
	} else {
		model, err = buildModelFromFiles(result.LocalPaths, modelFiles, configFiles, tempDir)
	}
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
		configArchive, err := createConfigArchive(localPaths, configFiles, tempDir)
		if err != nil {
			return nil, fmt.Errorf("create config archive: %w", err)
		}
		// Note: configArchive is created inside tempDir and will be cleaned up when
		// the caller removes tempDir. The file must exist until after store.Write()
		// completes since the model artifact references it lazily.

		if configArchive != "" {
			b, err = b.WithConfigArchive(configArchive)
			if err != nil {
				return nil, fmt.Errorf("add config archive: %w", err)
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

// buildDiffusersModel constructs an OCI model artifact for a diffusers model
func buildDiffusersModel(localPaths map[string]string, modelFiles, configFiles []RepoFile, tempDir string) (types.ModelArtifact, error) {
	// For diffusers models, we create a tar archive preserving the directory structure
	allFiles := append(modelFiles, configFiles...)

	// Create a tar archive of all files
	archivePath, err := createDiffusersArchive(localPaths, allFiles, tempDir)
	if err != nil {
		return nil, fmt.Errorf("create diffusers archive: %w", err)
	}

	// We still need safetensors files to create the base model
	// Find a safetensors file to use as the base
	var safetensorsPaths, binPaths []string
	for _, f := range modelFiles {
		lowerFilename := strings.ToLower(f.Filename())
		localPath, ok := localPaths[f.Path]
		if !ok {
			continue
		}
		if strings.HasSuffix(lowerFilename, ".safetensors") {
			safetensorsPaths = append(safetensorsPaths, localPath)
		} else if strings.HasSuffix(lowerFilename, ".bin") {
			binPaths = append(binPaths, localPath)
		}
	}

	if len(safetensorsPaths) == 0 {
		safetensorsPaths = binPaths
	}

	if len(safetensorsPaths) == 0 {
		return nil, fmt.Errorf("no model weight files found")
	}

	sort.Strings(safetensorsPaths)

	// Create builder from the first weight file to establish base
	// Note: We use the first file to establish the base, but the full archive will contain all files
	// The builder.FromSafetensors function handles both .safetensors and .bin files
	b, err := builder.FromSafetensors([]string{safetensorsPaths[0]})
	if err != nil {
		return nil, fmt.Errorf("create builder: %w", err)
	}

	// Add the diffusers archive as a directory tar layer
	// This will be extracted preserving the full directory structure
	b, err = b.WithDirTar(archivePath)
	if err != nil {
		return nil, fmt.Errorf("add diffusers archive: %w", err)
	}

	return b.Model(), nil
}

// createDiffusersArchive creates a tar archive of diffusers model files preserving directory structure
func createDiffusersArchive(localPaths map[string]string, files []RepoFile, tempDir string) (string, error) {
	archivePath := filepath.Join(tempDir, "diffusers-model.tar")

	f, err := os.Create(archivePath)
	if err != nil {
		return "", fmt.Errorf("create archive file: %w", err)
	}
	defer f.Close()

	tw := tar.NewWriter(f)
	defer tw.Close()

	for _, file := range files {
		localPath, ok := localPaths[file.Path]
		if !ok {
			log.Printf("Warning: skipping file %s (not downloaded)", file.Path)
			continue
		}

		// Add file to archive with its original path (preserving directory structure)
		if err := addFileToTar(tw, localPath, file.Path); err != nil {
			return "", fmt.Errorf("add file %s to archive: %w", file.Path, err)
		}
	}

	return archivePath, nil
}

// addFileToTar adds a file to a tar archive with the specified archive path
func addFileToTar(tw *tar.Writer, sourcePath, archivePath string) error {
	// Get file info
	info, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}

	// Create tar header
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return fmt.Errorf("create tar header: %w", err)
	}

	// Use the archive path (with forward slashes for tar)
	header.Name = filepath.ToSlash(archivePath)

	// Write header
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("write tar header: %w", err)
	}

	// If it's a file (not directory), write contents
	if !info.IsDir() {
		file, err := os.Open(sourcePath)
		if err != nil {
			return fmt.Errorf("open file: %w", err)
		}
		defer file.Close()

		if _, err := io.Copy(tw, file); err != nil {
			return fmt.Errorf("copy file contents: %w", err)
		}
	}

	return nil
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
