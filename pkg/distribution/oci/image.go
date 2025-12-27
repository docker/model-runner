package oci

// Image defines the interface for interacting with an OCI image.
type Image interface {
	// Layers returns the ordered collection of filesystem layers that comprise this image.
	// The order of the list is oldest/base layer first, and most-recent/top layer last.
	Layers() ([]Layer, error)

	// MediaType of this image's manifest.
	MediaType() (MediaType, error)

	// Size returns the size of the manifest.
	Size() (int64, error)

	// ConfigName returns the hash of the image's config file, also known as
	// the Image ID.
	ConfigName() (Hash, error)

	// ConfigFile returns this image's config file.
	ConfigFile() (*ConfigFile, error)

	// RawConfigFile returns the serialized bytes of ConfigFile().
	RawConfigFile() ([]byte, error)

	// Digest returns the sha256 of this image's manifest.
	Digest() (Hash, error)

	// Manifest returns this image's Manifest object.
	Manifest() (*Manifest, error)

	// RawManifest returns the serialized bytes of Manifest()
	RawManifest() ([]byte, error)

	// LayerByDigest returns a Layer for interacting with a particular layer of
	// the image, looking it up by "digest" (the compressed hash).
	LayerByDigest(Hash) (Layer, error)

	// LayerByDiffID is an analog to LayerByDigest, looking up by "diff id"
	// (the uncompressed hash).
	LayerByDiffID(Hash) (Layer, error)
}
