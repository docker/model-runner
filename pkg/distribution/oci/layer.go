package oci

import (
	"io"
)

// Layer is an interface for accessing the properties of a particular layer of an Image.
type Layer interface {
	// Digest returns the Hash of the compressed layer.
	Digest() (Hash, error)

	// DiffID returns the Hash of the uncompressed layer.
	DiffID() (Hash, error)

	// Compressed returns an io.ReadCloser for the compressed layer contents.
	Compressed() (io.ReadCloser, error)

	// Uncompressed returns an io.ReadCloser for the uncompressed layer contents.
	Uncompressed() (io.ReadCloser, error)

	// Size returns the compressed size of the Layer.
	Size() (int64, error)

	// MediaType returns the media type of the Layer.
	MediaType() (MediaType, error)
}
