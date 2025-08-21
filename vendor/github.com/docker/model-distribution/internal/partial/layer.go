package partial

import (
	"io"
	"os"

	"github.com/google/go-containerregistry/pkg/v1"
	ggcrtypes "github.com/google/go-containerregistry/pkg/v1/types"
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
	return &Layer{
		Path: path,
		Descriptor: v1.Descriptor{
			Size:      size,
			Digest:    hash,
			MediaType: mt,
		},
	}, nil
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
