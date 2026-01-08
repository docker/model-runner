package partial

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/docker/model-runner/pkg/distribution/types"
	v1 "github.com/docker/model-runner/pkg/go-containerregistry/pkg/v1"
	ggcrtypes "github.com/docker/model-runner/pkg/go-containerregistry/pkg/v1/types"
)

var _ v1.Layer = &Layer{}

// Layer represents a layer in a model distribution.
type Layer struct {
	Path string
	v1.Descriptor
}

// NewLayer creates a new layer from a file path and media type.
func NewLayer(path string, mt ggcrtypes.MediaType) (*Layer, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	hash, size, err := v1.SHA256(f)
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

	// Create annotations
	annotations := map[string]string{
		types.AnnotationFilePath:          filepath.Base(path),
		types.AnnotationFileMetadata:      string(metadataJSON),
		types.AnnotationMediaTypeUntested: "false", // Media types are tested in this implementation
	}

	return &Layer{
		Path: path,
		Descriptor: v1.Descriptor{
			Size:        size,
			Digest:      hash,
			MediaType:   mt,
			Annotations: annotations,
		},
	}, err
}

// Digest returns the layer's digest.
func (l Layer) Digest() (v1.Hash, error) {
	return l.DiffID()
}

// DiffID returns the layer's diff ID.
func (l Layer) DiffID() (v1.Hash, error) {
	return l.Descriptor.Digest, nil
}

// Compressed returns a reader for the compressed layer contents.
func (l Layer) Compressed() (io.ReadCloser, error) {
	return l.Uncompressed()
}

// Uncompressed returns a reader for the uncompressed layer contents.
func (l Layer) Uncompressed() (io.ReadCloser, error) {
	return os.Open(l.Path)
}

// Size returns the size of the layer.
func (l Layer) Size() (int64, error) {
	return l.Descriptor.Size, nil
}

// MediaType returns the media type of the layer.
func (l Layer) MediaType() (ggcrtypes.MediaType, error) {
	return l.Descriptor.MediaType, nil
}
