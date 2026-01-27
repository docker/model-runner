package partial

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
)

var _ oci.Layer = &Layer{}

type Layer struct {
	Path string
	oci.Descriptor
}

func NewLayer(path string, mt oci.MediaType) (*Layer, error) {
	return NewLayerWithRelativePath(path, filepath.Base(path), mt)
}

// NewLayerWithRelativePath creates a layer with a specific relative path stored in annotations.
// This is useful for preserving directory structure when packaging nested model files.
// The relativePath parameter specifies the path that will be used when unpacking the layer,
// allowing files from subdirectories to be correctly placed.
func NewLayerWithRelativePath(path string, relativePath string, mt oci.MediaType) (*Layer, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	hash, size, err := oci.SHA256(f)
	if err != nil {
		return nil, err
	}

	// Get file info for metadata
	fileInfo, err := f.Stat()
	if err != nil {
		return nil, err
	}

	// Create file metadata
	metadata := types.FileMetadata{
		Name:     filepath.Base(path),
		Mode:     uint32(fileInfo.Mode().Perm()),
		Uid:      0, // Default to 0 as os.FileInfo doesn't provide this on all platforms
		Gid:      0, // Default to 0 as os.FileInfo doesn't provide this on all platforms
		Size:     fileInfo.Size(),
		ModTime:  fileInfo.ModTime(),
		Typeflag: 0, // 0 for regular file (tar.TypeReg)
	}

	// Serialize metadata to JSON
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}

	// Use forward slashes for cross-platform compatibility in annotations
	annotationPath := filepath.ToSlash(relativePath)

	// Create annotations
	annotations := map[string]string{
		types.AnnotationFilePath:          annotationPath,
		types.AnnotationFileMetadata:      string(metadataJSON),
		types.AnnotationMediaTypeUntested: "false", // Media types are tested in this implementation
	}

	return &Layer{
		Path: path,
		Descriptor: oci.Descriptor{
			Size:        size,
			Digest:      hash,
			MediaType:   mt,
			Annotations: annotations,
		},
	}, err
}

func (l Layer) Digest() (oci.Hash, error) {
	return l.DiffID()
}

func (l Layer) DiffID() (oci.Hash, error) {
	return l.Descriptor.Digest, nil
}

func (l Layer) Compressed() (io.ReadCloser, error) {
	return l.Uncompressed()
}

func (l Layer) Uncompressed() (io.ReadCloser, error) {
	return os.Open(l.Path)
}

func (l Layer) Size() (int64, error) {
	return l.Descriptor.Size, nil
}

func (l Layer) MediaType() (oci.MediaType, error) {
	return l.Descriptor.MediaType, nil
}
