package partial

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/v1"
	ggcrtypes "github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/docker/model-runner/pkg/distribution/types"
)

var _ v1.Layer = &Layer{}

type Layer struct {
	Path string
	v1.Descriptor
}

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

func (l Layer) Digest() (v1.Hash, error) {
	return l.DiffID()
}

func (l Layer) DiffID() (v1.Hash, error) {
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

func (l Layer) MediaType() (ggcrtypes.MediaType, error) {
	return l.Descriptor.MediaType, nil
}
