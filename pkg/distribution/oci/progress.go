package oci

// Update represents a progress update during image operations.
type Update struct {
	Complete int64
	Total    int64
	Error    error
}
