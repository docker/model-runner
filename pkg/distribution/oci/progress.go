package oci

// UploadingLayerID is a sentinel layer ID used to identify push operations.
// During push, there is no real layer available yet, so this fake ID signals
// that the operation is an upload rather than a download.
const UploadingLayerID = "uploading"

// Update represents a progress update during image operations.
type Update struct {
	Complete int64
	Total    int64
	Error    error
}
