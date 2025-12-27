package bundle

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
)

// Unpack creates and return a Bundle by unpacking files and config from model into dir.
func Unpack(dir string, model types.Model) (*Bundle, error) {
	bundle := &Bundle{
		dir: dir,
	}

	// Create model subdirectory upfront - all unpack operations will use it
	modelDir := filepath.Join(bundle.dir, ModelSubdir)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		return nil, fmt.Errorf("create model directory: %w", err)
	}

	// Inspect layers to determine what to unpack
	modelFormat := detectModelFormat(model)

	// Unpack model weights based on detected format
	switch modelFormat {
	case types.FormatGGUF:
		if err := unpackGGUFs(bundle, model); err != nil {
			return nil, fmt.Errorf("unpack GGUF files: %w", err)
		}
	case types.FormatSafetensors:
		if err := unpackSafetensors(bundle, model); err != nil {
			return nil, fmt.Errorf("unpack safetensors files: %w", err)
		}
	default:
		return nil, fmt.Errorf("no supported model weights found (neither GGUF nor safetensors)")
	}

	// Unpack optional components based on their presence
	if hasLayerWithMediaType(model, types.MediaTypeMultimodalProjector) {
		if err := unpackMultiModalProjector(bundle, model); err != nil {
			return nil, fmt.Errorf("add multi-model projector file to runtime bundle: %w", err)
		}
	}

	if hasLayerWithMediaType(model, types.MediaTypeChatTemplate) {
		if err := unpackTemplate(bundle, model); err != nil {
			return nil, fmt.Errorf("add chat template file to runtime bundle: %w", err)
		}
	}

	if hasLayerWithMediaType(model, types.MediaTypeVLLMConfigArchive) {
		if err := unpackConfigArchive(bundle, model); err != nil {
			return nil, fmt.Errorf("add config archive to runtime bundle: %w", err)
		}
	}

	// Unpack directory tar archives (can be multiple)
	if err := unpackDirTarArchives(bundle, model); err != nil {
		return nil, fmt.Errorf("unpack directory tar archives: %w", err)
	}

	// Always create the runtime config
	if err := unpackRuntimeConfig(bundle, model); err != nil {
		return nil, fmt.Errorf("add config.json to runtime bundle: %w", err)
	}

	return bundle, nil
}

// detectModelFormat inspects the model to determine the primary model format
func detectModelFormat(model types.Model) types.Format {
	// Check for GGUF files
	ggufPaths, err := model.GGUFPaths()
	if err == nil && len(ggufPaths) > 0 {
		return types.FormatGGUF
	}

	// Check for Safetensors files
	safetensorsPaths, err := model.SafetensorsPaths()
	if err == nil && len(safetensorsPaths) > 0 {
		return types.FormatSafetensors
	}

	return ""
}

// hasLayerWithMediaType checks if the model contains a layer with the specified media type
func hasLayerWithMediaType(model types.Model, targetMediaType oci.MediaType) bool {
	// Check specific media types using the model's methods
	//nolint:exhaustive // only checking for specific layer types
	switch targetMediaType {
	case types.MediaTypeMultimodalProjector:
		path, err := model.MMPROJPath()
		return err == nil && path != ""
	case types.MediaTypeChatTemplate:
		path, err := model.ChatTemplatePath()
		return err == nil && path != ""
	case types.MediaTypeVLLMConfigArchive:
		path, err := model.ConfigArchivePath()
		return err == nil && path != ""
	default:
		return false
	}
}

func unpackRuntimeConfig(bundle *Bundle, mdl types.Model) error {
	cfg, err := mdl.Config()
	if err != nil {
		return err
	}

	// Runtime config stays at bundle root
	f, err := os.Create(filepath.Join(bundle.dir, "config.json"))
	if err != nil {
		return fmt.Errorf("create runtime config file: %w", err)
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(cfg); err != nil {
		return fmt.Errorf("encode runtime config: %w", err)
	}
	bundle.runtimeConfig = cfg
	return nil
}

func unpackGGUFs(bundle *Bundle, mdl types.Model) error {
	ggufPaths, err := mdl.GGUFPaths()
	if err != nil {
		return fmt.Errorf("get GGUF files for model: %w", err)
	}

	modelDir := filepath.Join(bundle.dir, ModelSubdir)

	if len(ggufPaths) == 1 {
		if err := unpackFile(filepath.Join(modelDir, "model.gguf"), ggufPaths[0]); err != nil {
			return err
		}
		bundle.ggufFile = "model.gguf"
		return err
	}

	for i := range ggufPaths {
		name := fmt.Sprintf("model-%05d-of-%05d.gguf", i+1, len(ggufPaths))
		if err := unpackFile(filepath.Join(modelDir, name), ggufPaths[i]); err != nil {
			return err
		}
		bundle.ggufFile = name
	}

	return nil
}

func unpackMultiModalProjector(bundle *Bundle, mdl types.Model) error {
	path, err := mdl.MMPROJPath()
	if err != nil {
		return nil // no such file
	}

	modelDir := filepath.Join(bundle.dir, ModelSubdir)

	if err = unpackFile(filepath.Join(modelDir, "model.mmproj"), path); err != nil {
		return err
	}
	bundle.mmprojPath = "model.mmproj"
	return nil
}

func unpackTemplate(bundle *Bundle, mdl types.Model) error {
	path, err := mdl.ChatTemplatePath()
	if err != nil {
		return nil // no such file
	}

	modelDir := filepath.Join(bundle.dir, ModelSubdir)

	if err = unpackFile(filepath.Join(modelDir, "template.jinja"), path); err != nil {
		return err
	}
	bundle.chatTemplatePath = "template.jinja"
	return nil
}

func unpackSafetensors(bundle *Bundle, mdl types.Model) error {
	safetensorsPaths, err := mdl.SafetensorsPaths()
	if err != nil {
		return fmt.Errorf("get safetensors files for model: %w", err)
	}

	if len(safetensorsPaths) == 0 {
		return fmt.Errorf("no safetensors files found")
	}

	modelDir := filepath.Join(bundle.dir, ModelSubdir)

	if len(safetensorsPaths) == 1 {
		if err := unpackFile(filepath.Join(modelDir, "model.safetensors"), safetensorsPaths[0]); err != nil {
			return err
		}
		bundle.safetensorsFile = "model.safetensors"
		return nil
	}

	// Handle sharded safetensors files
	for i := range safetensorsPaths {
		name := fmt.Sprintf("model-%05d-of-%05d.safetensors", i+1, len(safetensorsPaths))
		if err := unpackFile(filepath.Join(modelDir, name), safetensorsPaths[i]); err != nil {
			return err
		}
		if i == 0 {
			bundle.safetensorsFile = name
		}
	}

	return nil
}

func unpackConfigArchive(bundle *Bundle, mdl types.Model) error {
	archivePath, err := mdl.ConfigArchivePath()
	if err != nil {
		return fmt.Errorf("get config archive path: %w", err)
	}

	modelDir := filepath.Join(bundle.dir, ModelSubdir)

	// Extract the tar archive into the model subdirectory
	// This prevents config.json conflicts with the runtime config at bundle root
	if err := extractTarArchive(archivePath, modelDir); err != nil {
		return fmt.Errorf("extract config archive: %w", err)
	}

	return nil
}

func unpackDirTarArchives(bundle *Bundle, mdl types.Model) error {
	// Cast to ModelArtifact to access Layers() method
	artifact, ok := mdl.(types.ModelArtifact)
	if !ok {
		// If it's not a ModelArtifact, there are no layers to extract
		return nil
	}

	// Get all layers from the model
	layers, err := artifact.Layers()
	if err != nil {
		return fmt.Errorf("get model layers: %w", err)
	}

	modelDir := filepath.Join(bundle.dir, ModelSubdir)

	// Iterate through layers and extract directory tar archives
	for _, layer := range layers {
		mediaType, err := layer.MediaType()
		if err != nil {
			fmt.Printf("Warning: failed to get media type for layer: %v", err)
			continue
		}

		// Check if this is a directory tar layer
		if mediaType != types.MediaTypeDirTar {
			continue
		}

		// Get the layer as an uncompressed stream (decompression handled automatically)
		uncompressed, err := layer.Uncompressed()
		if err != nil {
			return fmt.Errorf("get uncompressed layer: %w", err)
		}

		// Stream directly to tar extraction - no temp file needed
		if err := extractTarArchiveFromReader(uncompressed, modelDir); err != nil {
			uncompressed.Close()
			return fmt.Errorf("extract directory tar archive: %w", err)
		}
		uncompressed.Close()
	}

	return nil
}

// validatePathWithinDirectory checks if targetPath is within baseDir to prevent directory traversal attacks.
// It uses filepath.IsLocal() to provide robust security against
// various directory traversal attempts including edge cases like empty paths, ".", "..", symbolic links, etc.
func validatePathWithinDirectory(baseDir, targetPath string) error {
	// Get absolute path of base directory
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("get absolute base directory path: %w", err)
	}

	// Construct the target path within base directory
	target := filepath.Join(absBaseDir, targetPath)

	// Get absolute path of target
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("get absolute target path: %w", err)
	}

	// Get relative path from base to target
	rel, err := filepath.Rel(absBaseDir, absTarget)
	if err != nil {
		return fmt.Errorf("compute relative path: %w", err)
	}

	// Use filepath.IsLocal() to check if the relative path is local (doesn't escape baseDir)
	// This handles all edge cases including empty strings, ".", "..", symlinks, etc.
	if !filepath.IsLocal(rel) {
		return fmt.Errorf("invalid entry %q: path attempts to escape destination directory", targetPath)
	}

	return nil
}

func extractTarArchiveFromReader(r io.Reader, destDir string) error {
	// Get absolute path of destination directory for security checks
	absDestDir, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("get absolute destination path: %w", err)
	}

	// Create tar reader
	tr := tar.NewReader(r)

	// Extract files
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return fmt.Errorf("read tar header: %w", err)
		}

		// Validate the target path to prevent directory traversal
		if err := validatePathWithinDirectory(absDestDir, header.Name); err != nil {
			return err
		}

		// Clean the path first to resolve any .. or . components
		cleanName := filepath.Clean(header.Name)
		if strings.HasPrefix(cleanName, "..") || filepath.IsAbs(cleanName) {
			return fmt.Errorf("invalid file path in archive: %s", header.Name)
		}
		// Construct the validated target path
		absTarget := filepath.Join(absDestDir, cleanName)

		// Process based on header type
		switch header.Typeflag {
		case tar.TypeDir:
			// Create directory
			if err := os.MkdirAll(absTarget, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("create directory %s: %w", absTarget, err)
			}

		case tar.TypeReg:
			// Extract regular file
			if err := extractFile(tr, absTarget, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("extract file %s: %w", absTarget, err)
			}

		case tar.TypeSymlink:
			// Skip symlinks - not needed for model distribution
			// Symlinks could enable directory traversal attacks even with validation
			// Model archives should only contain regular files and directories
			continue

		default:
			// Skip other types (block devices, char devices, FIFOs, etc.)
			continue
		}
	}

	return nil
}

func extractTarArchive(archivePath, destDir string) error {
	// Open the tar file
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open tar archive: %w", err)
	}
	defer file.Close()

	// Delegate to the streaming version
	return extractTarArchiveFromReader(file, destDir)
}

// extractFile extracts a single file from the tar reader
func extractFile(tr io.Reader, target string, mode os.FileMode) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}

	// Create the file
	file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer file.Close()

	// Copy contents
	if _, err := io.Copy(file, tr); err != nil {
		return fmt.Errorf("write file contents: %w", err)
	}

	return nil
}

func unpackFile(bundlePath string, srcPath string) error {
	return os.Link(srcPath, bundlePath)
}
