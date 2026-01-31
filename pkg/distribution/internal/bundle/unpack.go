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
// It auto-detects the packaging version:
//   - V0.2 (layer-per-file with annotations): Uses UnpackFromLayers for full path preservation
//   - V0.1 (legacy): Uses the original unpacking logic based on GGUFPaths(), SafetensorsPaths(), etc.
func Unpack(dir string, model types.Model) (*Bundle, error) {
	artifact, isArtifact := model.(types.ModelArtifact)
	if isArtifact && isV02Model(artifact) {
		return UnpackFromLayers(dir, artifact)
	}

	// V0.1 legacy unpacking
	return unpackLegacy(dir, model)
}

// isV02Model checks if the model was packaged using V0.2 format (layer-per-file with annotations).
// It does this by checking the config media type in the manifest.
func isV02Model(model types.ModelArtifact) bool {
	manifest, err := model.Manifest()
	if err != nil {
		return false
	}
	return manifest.Config.MediaType == types.MediaTypeModelConfigV02
}

// unpackLegacy is the original V0.1 unpacking logic that uses model.GGUFPaths(), model.SafetensorsPaths(), etc.
func unpackLegacy(dir string, model types.Model) (*Bundle, error) {
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
	case types.FormatDiffusers:
		if err := unpackDDUF(bundle, model); err != nil {
			return nil, fmt.Errorf("unpack DDUF file: %w", err)
		}
	default:
		return nil, fmt.Errorf("no supported model weights found (expected GGUF, safetensors, or diffusers/DDUF)")
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

	// Unpack generic file layers (new format - each file as individual layer with annotation)
	if err := unpackGenericFileLayers(bundle, model); err != nil {
		return nil, fmt.Errorf("unpack generic file layers: %w", err)
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

	// Check for DDUF files
	ddufPaths, err := model.DDUFPaths()
	if err == nil && len(ddufPaths) > 0 {
		return types.FormatDiffusers
	}

	return ""
}

// unpackDDUF unpacks a DDUF (Diffusers Unified Format) file to the bundle.
func unpackDDUF(bundle *Bundle, mdl types.Model) error {
	ddufPaths, err := mdl.DDUFPaths()
	if err != nil {
		return fmt.Errorf("get DDUF files for model: %w", err)
	}

	if len(ddufPaths) == 0 {
		return fmt.Errorf("no DDUF files found")
	}

	modelDir := filepath.Join(bundle.dir, ModelSubdir)

	// DDUF is a single-file format
	ddufFilename := filepath.Base(ddufPaths[0])
	// Ensure the filename has the .dduf extension for proper detection by diffusers server
	if !strings.HasSuffix(strings.ToLower(ddufFilename), ".dduf") {
		ddufFilename = ddufFilename + ".dduf"
	}
	if err := unpackFile(filepath.Join(modelDir, ddufFilename), ddufPaths[0]); err != nil {
		return err
	}
	bundle.ddufFile = ddufFilename
	return nil
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

// configProvider is an interface for types that provide a Config() method.
// Both types.Model and types.ModelArtifact satisfy this interface.
type configProvider interface {
	Config() (types.ModelConfig, error)
}

func unpackRuntimeConfig(bundle *Bundle, mdl configProvider) error {
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

	// Try to use filepath annotations from the model layers if available
	artifact, ok := mdl.(types.ModelArtifact)
	if ok {
		layers, layerErr := artifact.Layers()
		if layerErr == nil {
			return unpackSafetensorsWithAnnotations(bundle, modelDir, safetensorsPaths, layers)
		}
	}

	// Fall back to legacy behavior (hardcoded names)
	return unpackSafetensorsLegacy(bundle, modelDir, safetensorsPaths)
}

// unpackSafetensorsWithAnnotations unpacks safetensors files using the filepath annotation
// from each layer. This allows preserving nested directory structure.
func unpackSafetensorsWithAnnotations(bundle *Bundle, modelDir string, safetensorsPaths []string, layers []oci.Layer) error {
	// Build a map of blob path -> layer annotation filepath
	blobToFilepath := make(map[string]string)
	for _, layer := range layers {
		mt, err := layer.MediaType()
		if err != nil || mt != types.MediaTypeSafetensors {
			continue
		}

		// Get the layer's digest
		digest, err := layer.Digest()
		if err != nil {
			continue
		}

		// Try to get annotations - need to check if layer has Descriptor embedded
		// This works with partial.Layer type which embeds oci.Descriptor
		type descriptorProvider interface {
			GetDescriptor() oci.Descriptor
		}
		if dp, ok := layer.(descriptorProvider); ok {
			desc := dp.GetDescriptor()
			if fp, exists := desc.Annotations[types.AnnotationFilePath]; exists {
				blobToFilepath[digest.Hex] = fp
			}
		}
	}

	// Check if we have any annotations - if not, fall back to legacy
	if len(blobToFilepath) == 0 {
		return unpackSafetensorsLegacy(bundle, modelDir, safetensorsPaths)
	}

	// Unpack each safetensors file using its annotation
	for i, srcPath := range safetensorsPaths {
		// Extract digest hex from path (paths are like /path/to/blobs/sha256/<hex>)
		digestHex := filepath.Base(srcPath)

		var destRelPath string
		if annotatedPath, ok := blobToFilepath[digestHex]; ok {
			// Use the annotated path
			destRelPath = annotatedPath
		} else {
			// No annotation found - use legacy naming
			if len(safetensorsPaths) == 1 {
				destRelPath = "model.safetensors"
			} else {
				destRelPath = fmt.Sprintf("model-%05d-of-%05d.safetensors", i+1, len(safetensorsPaths))
			}
		}

		// SECURITY: Validate the path to prevent directory traversal attacks
		// This blocks paths like "../../../etc/passwd" or "/etc/passwd"
		if err := validatePathWithinDirectory(modelDir, destRelPath); err != nil {
			return fmt.Errorf("invalid filepath annotation %q: %w", destRelPath, err)
		}

		// Convert forward slashes to OS-specific separator
		destRelPath = filepath.FromSlash(destRelPath)
		destPath := filepath.Join(modelDir, destRelPath)

		// Create parent directories if needed
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("create parent directory for %s: %w", destRelPath, err)
		}

		if err := unpackFile(destPath, srcPath); err != nil {
			return err
		}

		// Track the first file for bundle reference
		if i == 0 {
			bundle.safetensorsFile = destRelPath
		}
	}

	return nil
}

// unpackSafetensorsLegacy unpacks safetensors files using hardcoded naming.
// This is the fallback for models packaged without filepath annotations.
func unpackSafetensorsLegacy(bundle *Bundle, modelDir string, safetensorsPaths []string) error {
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
// It performs multiple security checks:
// 1. Rejects empty paths
// 2. Rejects paths containing null bytes
// 3. Rejects absolute paths (must be relative)
// 4. Rejects paths that are just "." (current directory)
// 5. Uses filepath.IsLocal() to reject paths that escape the base directory
func validatePathWithinDirectory(baseDir, targetPath string) error {
	// SECURITY: Reject empty paths
	if targetPath == "" {
		return fmt.Errorf("invalid entry: empty path is not allowed")
	}

	// SECURITY: Reject paths containing null bytes (can bypass security checks in some systems)
	if strings.ContainsRune(targetPath, 0) {
		return fmt.Errorf("invalid entry %q: path contains null byte", targetPath)
	}

	// SECURITY: Reject absolute paths - we only accept relative paths
	// This handles both Unix (/etc/passwd) and Windows (C:\Windows) absolute paths
	if filepath.IsAbs(targetPath) {
		return fmt.Errorf("invalid entry %q: absolute paths are not allowed", targetPath)
	}

	// SECURITY: Reject paths that are just "." - this would write to the directory itself
	// which doesn't make sense for extracting files
	cleanPath := filepath.Clean(targetPath)
	if cleanPath == "." {
		return fmt.Errorf("invalid entry %q: path resolves to current directory", targetPath)
	}

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
	// This handles edge cases including symlinks, "..", etc.
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

// UnpackFromLayers unpacks a model that was packaged using the layer-per-file approach.
// Each file is stored as an individual layer with its filepath preserved in annotations.
// This is the approach used by builder.FromDirectory and preserves nested directory structure.
//
// Unlike the standard Unpack function which uses model.GGUFPaths(), model.SafetensorsPaths(), etc.,
// this function iterates directly over layers and uses their filepath annotations.
func UnpackFromLayers(dir string, model types.ModelArtifact) (*Bundle, error) {
	bundle := &Bundle{
		dir: dir,
	}

	// Create model subdirectory upfront - all unpack operations will use it
	modelDir := filepath.Join(bundle.dir, ModelSubdir)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		return nil, fmt.Errorf("create model directory: %w", err)
	}

	// Get all layers from the model
	layers, err := model.Layers()
	if err != nil {
		return nil, fmt.Errorf("get model layers: %w", err)
	}

	// Define the interface for getting descriptor with annotations
	type descriptorProvider interface {
		GetDescriptor() oci.Descriptor
	}

	// Iterate through all layers and unpack using annotations
	for _, layer := range layers {
		mediaType, err := layer.MediaType()
		if err != nil {
			continue
		}

		// Get the filepath annotation
		dp, ok := layer.(descriptorProvider)
		if !ok {
			continue
		}

		desc := dp.GetDescriptor()
		relPath, exists := desc.Annotations[types.AnnotationFilePath]
		if !exists || relPath == "" {
			continue
		}

		// Validate the path to prevent directory traversal
		if err := validatePathWithinDirectory(modelDir, relPath); err != nil {
			return nil, fmt.Errorf("invalid filepath annotation %q: %w", relPath, err)
		}

		// Convert forward slashes to OS-specific separator
		relPath = filepath.FromSlash(relPath)
		destPath := filepath.Join(modelDir, relPath)

		// Skip if file already exists
		if _, err := os.Stat(destPath); err == nil {
			continue
		}

		// Create parent directories if needed
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return nil, fmt.Errorf("create parent directory for %s: %w", relPath, err)
		}

		// Unpack the file
		if err := unpackLayerToFile(destPath, layer); err != nil {
			return nil, fmt.Errorf("unpack %s: %w", relPath, err)
		}

		// Update bundle tracking fields
		updateBundleFieldsFromLayer(bundle, mediaType, relPath)
	}

	// Create the runtime config
	if err := unpackRuntimeConfig(bundle, model); err != nil {
		return nil, fmt.Errorf("add config.json to runtime bundle: %w", err)
	}

	return bundle, nil
}

// unpackLayerToFile unpacks a single layer to the destination path using hard linking.
// It requires the layer to be a local layer with a file path (pathProvider interface).
func unpackLayerToFile(destPath string, layer oci.Layer) error {
	// Try to get the layer's local path for hard linking
	type pathProvider interface {
		GetPath() string
	}

	if pp, ok := layer.(pathProvider); ok {
		// Use hard link for local layers
		return unpackFile(destPath, pp.GetPath())
	}
	return fmt.Errorf("layer is not a path provider")
}

// updateBundleFieldsFromLayer updates the bundle tracking fields based on the unpacked layer.
func updateBundleFieldsFromLayer(bundle *Bundle, mediaType oci.MediaType, relPath string) {
	//nolint:exhaustive // only tracking specific model-related media types
	switch mediaType {
	case types.MediaTypeGGUF:
		if bundle.ggufFile == "" {
			bundle.ggufFile = relPath
		}
	case types.MediaTypeSafetensors:
		if bundle.safetensorsFile == "" {
			bundle.safetensorsFile = relPath
		}
	case types.MediaTypeDDUF:
		if bundle.ddufFile == "" {
			bundle.ddufFile = relPath
		}
	case types.MediaTypeMultimodalProjector:
		if bundle.mmprojPath == "" {
			bundle.mmprojPath = relPath
		}
	case types.MediaTypeChatTemplate:
		if bundle.chatTemplatePath == "" {
			bundle.chatTemplatePath = relPath
		}
	}
}

// unpackGenericFileLayers unpacks layers with MediaTypeModelFile using their filepath annotation.
// This supports the new format where each config file is packaged as an individual layer
// with its relative path preserved in the annotation.
func unpackGenericFileLayers(bundle *Bundle, mdl types.Model) error {
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

	// Define the interface for getting descriptor with annotations
	type descriptorProvider interface {
		GetDescriptor() oci.Descriptor
	}

	// Iterate through layers and extract generic file layers
	for _, layer := range layers {
		mediaType, err := layer.MediaType()
		if err != nil {
			continue
		}

		// Only process generic model file layers
		if mediaType != types.MediaTypeModelFile {
			continue
		}

		// Get the filepath annotation
		dp, ok := layer.(descriptorProvider)
		if !ok {
			continue
		}

		desc := dp.GetDescriptor()
		relPath, exists := desc.Annotations[types.AnnotationFilePath]
		if !exists || relPath == "" {
			continue
		}

		// Validate the path to prevent directory traversal
		if err := validatePathWithinDirectory(modelDir, relPath); err != nil {
			return fmt.Errorf("invalid filepath annotation %q: %w", relPath, err)
		}

		// Convert forward slashes to OS-specific separator
		relPath = filepath.FromSlash(relPath)
		destPath := filepath.Join(modelDir, relPath)

		// Skip if file already exists (might have been unpacked by another method)
		if _, err := os.Stat(destPath); err == nil {
			continue
		}

		// Create parent directories if needed
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("create parent directory for %s: %w", relPath, err)
		}

		// Try to get the layer's local path for hard linking
		// Use interface to check if layer has a Path field (like partial.Layer)
		type pathProvider interface {
			GetPath() string
		}

		if pp, ok := layer.(pathProvider); ok {
			// Use hard link for local layers
			if err := unpackFile(destPath, pp.GetPath()); err != nil {
				return fmt.Errorf("unpack file %s: %w", relPath, err)
			}
		} else {
			// Fallback: copy from uncompressed stream (for remote layers)
			uncompressed, err := layer.Uncompressed()
			if err != nil {
				return fmt.Errorf("get uncompressed layer for %s: %w", relPath, err)
			}

			// Create the file
			destFile, err := os.Create(destPath)
			if err != nil {
				uncompressed.Close()
				return fmt.Errorf("create file %s: %w", relPath, err)
			}

			_, copyErr := io.Copy(destFile, uncompressed)
			destFile.Close()
			uncompressed.Close()

			if copyErr != nil {
				return fmt.Errorf("copy file %s: %w", relPath, copyErr)
			}
		}
	}

	return nil
}
