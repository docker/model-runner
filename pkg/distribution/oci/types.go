package oci

// MediaType is an enumeration of the supported mime types that an element of an image might have.
type MediaType string

// Common media types used in OCI and Docker image specifications.
const (
	// OCI manifest types
	OCIManifestSchema1   MediaType = "application/vnd.oci.image.manifest.v1+json"
	OCIImageIndex        MediaType = "application/vnd.oci.image.index.v1+json"
	OCIConfigJSON        MediaType = "application/vnd.oci.image.config.v1+json"
	OCILayer             MediaType = "application/vnd.oci.image.layer.v1.tar"
	OCILayerGzip         MediaType = "application/vnd.oci.image.layer.v1.tar+gzip"
	OCILayerZstd         MediaType = "application/vnd.oci.image.layer.v1.tar+zstd"
	OCIUncompressedLayer MediaType = "application/vnd.oci.image.layer.v1.tar"
	OCIContentDescriptor MediaType = "application/vnd.oci.descriptor.v1+json"
	OCIArtifactManifest  MediaType = "application/vnd.oci.artifact.manifest.v1+json"
	OCIEmptyJSON         MediaType = "application/vnd.oci.empty.v1+json"

	// Docker manifest types
	DockerManifestSchema2   MediaType = "application/vnd.docker.distribution.manifest.v2+json"
	DockerManifestList      MediaType = "application/vnd.docker.distribution.manifest.list.v2+json"
	DockerConfigJSON        MediaType = "application/vnd.docker.container.image.v1+json"
	DockerLayer             MediaType = "application/vnd.docker.image.rootfs.diff.tar.gzip"
	DockerForeignLayer      MediaType = "application/vnd.docker.image.rootfs.foreign.diff.tar.gzip"
	DockerUncompressedLayer MediaType = "application/vnd.docker.image.rootfs.diff.tar"
)

// IsDistributable returns true if a layer is distributable (not foreign).
func (m MediaType) IsDistributable() bool {
	return m != DockerForeignLayer
}

// IsImage returns true if the media type is a manifest type.
func (m MediaType) IsImage() bool {
	//nolint:exhaustive // only checking for specific manifest types
	switch m {
	case OCIManifestSchema1, DockerManifestSchema2:
		return true
	default:
		return false
	}
}

// IsIndex returns true if the media type is an index type.
func (m MediaType) IsIndex() bool {
	//nolint:exhaustive // only checking for specific index types
	switch m {
	case OCIImageIndex, DockerManifestList:
		return true
	default:
		return false
	}
}
