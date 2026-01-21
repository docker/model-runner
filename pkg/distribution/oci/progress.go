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

// MessageType represents the type of progress message
type MessageType string

const (
	// TypeProgress indicates a progress update message
	TypeProgress MessageType = "progress"
	// TypeSuccess indicates a successful completion message
	TypeSuccess MessageType = "success"
	// TypeWarning indicates a warning message
	TypeWarning MessageType = "warning"
	// TypeError indicates an error message
	TypeError MessageType = "error"
)

// Mode represents the operation mode (pull or push)
type Mode string

const (
	// ModePull indicates a pull operation
	ModePull Mode = "pull"
	// ModePush indicates a push operation
	ModePush Mode = "push"
)

// ProgressLayer represents layer information in a progress message
type ProgressLayer struct {
	ID      string `json:"id,omitempty"` // Layer ID
	Size    uint64 `json:"size"`         // Layer size
	Current uint64 `json:"current"`      // Current bytes transferred
}

// ProgressMessage represents a structured message for progress reporting
type ProgressMessage struct {
	Type    MessageType   `json:"type"`    // Message type: progress, success, warning, or error
	Message string        `json:"message"` // Deprecated for progress/success messages (clients should format based on Total/Layer). Still used for warnings and errors.
	Total   uint64        `json:"total"`
	Layer   ProgressLayer `json:"layer"` // Current layer information
	Mode    Mode          `json:"mode"`  // Operation mode: push or pull
}
