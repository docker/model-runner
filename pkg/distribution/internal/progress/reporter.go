package progress

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/docker/model-runner/pkg/distribution/oci"
)

// UpdateInterval defines how often progress updates should be sent
const UpdateInterval = 100 * time.Millisecond

// MinBytesForUpdate defines the minimum number of bytes that need to be transferred
// before sending a progress update
const MinBytesForUpdate = 1024 * 1024 // 1MB

type Layer struct {
	ID      string // Layer ID
	Size    uint64 // Layer size
	Current uint64 // Current bytes transferred
}

// Message represents a structured message for progress reporting
type Message struct {
	Type    string `json:"type"`    // "progress", "success", "warning", or "error"
	Message string `json:"message"` // Deprecated: the message should be defined by clients based on Message.Total and Message.Layer
	Total   uint64 `json:"total"`
	Pulled  uint64 `json:"pulled"` // Deprecated: use Layer.Current
	Layer   Layer  `json:"layer"`  // Current layer information
	Mode    string `json:"mode"`   // "push", "pull"
}

type Reporter struct {
	progress  chan oci.Update
	done      chan struct{}
	err       error
	out       io.Writer
	format    progressF
	layer     oci.Layer
	imageSize uint64
	mode      string
}

type progressF func(update oci.Update) string

func PullMsg(update oci.Update) string {
	return fmt.Sprintf("Downloaded: %.2f MB", float64(update.Complete)/1024/1024)
}

func PushMsg(update oci.Update) string {
	return fmt.Sprintf("Uploaded: %.2f MB", float64(update.Complete)/1024/1024)
}

func NewProgressReporter(w io.Writer, msgF progressF, imageSize int64, layer oci.Layer, mode string) *Reporter {
	return &Reporter{
		out:       w,
		progress:  make(chan oci.Update, 1),
		done:      make(chan struct{}),
		format:    msgF,
		layer:     layer,
		imageSize: safeUint64(imageSize),
		mode:      mode,
	}
}

// safeUint64 converts an int64 to uint64, ensuring the value is non-negative
func safeUint64(n int64) uint64 {
	if n < 0 {
		return 0
	}
	return uint64(n)
}

// Updates returns a channel for receiving progress Updates. It is the responsibility of the caller to close
// the channel when they are done sending Updates. Should only be called once per Reporter instance.
func (r *Reporter) Updates() chan<- oci.Update {
	go func() {
		var lastComplete int64
		var lastUpdate time.Time

		for p := range r.progress {
			if r.out == nil || r.err != nil {
				continue // If we fail to write progress, don't try again
			}
			now := time.Now()
			var layerSize uint64
			var layerID string
			if r.layer != nil {
				id, err := r.layer.DiffID()
				if err != nil {
					r.err = err
					continue
				}
				layerID = id.String()
				size, err := r.layer.Size()
				if err != nil {
					r.err = err
					continue
				}
				layerSize = safeUint64(size)
			}
			incrementalBytes := p.Complete - lastComplete

			// Only update if enough time has passed or enough bytes downloaded or finished
			if now.Sub(lastUpdate) >= UpdateInterval ||
				incrementalBytes >= MinBytesForUpdate ||
				safeUint64(p.Complete) == layerSize {
				if err := WriteProgress(r.out, r.format(p), r.imageSize, layerSize, safeUint64(p.Complete), layerID, r.mode); err != nil {
					r.err = err
				}
				lastUpdate = now
				lastComplete = p.Complete
			}
		}
		close(r.done) // Close the done channel when progress is complete
	}()
	return r.progress
}

// Wait waits for the progress Reporter to finish and returns any error encountered.
func (r *Reporter) Wait() error {
	<-r.done
	return r.err
}

// WriteProgress writes a progress update message
func WriteProgress(w io.Writer, msg string, imageSize, layerSize, current uint64, layerID string, mode string) error {
	return write(w, Message{
		Type:    "progress",
		Message: msg,
		Total:   imageSize,
		Pulled:  current,
		Layer: Layer{
			ID:      layerID,
			Size:    layerSize,
			Current: current,
		},
		Mode: mode,
	})
}

// WriteSuccess writes a success message
func WriteSuccess(w io.Writer, message string) error {
	return write(w, Message{
		Type:    "success",
		Message: message,
	})
}

// WriteError writes an error message
func WriteError(w io.Writer, message string) error {
	return write(w, Message{
		Type:    "error",
		Message: message,
	})
}

// WriteWarning writes a warning message
func WriteWarning(w io.Writer, message string) error {
	return write(w, Message{
		Type:    "warning",
		Message: message,
	})
}

// write writes a JSON-formatted progress message to the writer
func write(w io.Writer, msg Message) error {
	if w == nil {
		return nil
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", data)
	return err
}
