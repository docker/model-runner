package progress

import (
	"io"

	"github.com/docker/model-runner/pkg/distribution/oci"
)

// Reader wraps an io.Reader to track reading progress
type Reader struct {
	Reader       io.Reader
	ProgressChan chan<- oci.Update
	Total        int64
}

// NewReader returns a reader that reports progress to the given channel while reading.
func NewReader(r io.Reader, updates chan<- oci.Update) io.Reader {
	if updates == nil {
		return r
	}
	return &Reader{
		Reader:       r,
		ProgressChan: updates,
	}
}

// NewReaderWithOffset returns a reader that reports progress starting from an initial offset.
// This is useful for resuming interrupted downloads.
func NewReaderWithOffset(r io.Reader, updates chan<- oci.Update, initialOffset int64) io.Reader {
	if updates == nil {
		return r
	}
	return &Reader{
		Reader:       r,
		ProgressChan: updates,
		Total:        initialOffset,
	}
}

func (pr *Reader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	pr.Total += int64(n)
	if err == io.EOF {
		pr.ProgressChan <- oci.Update{Complete: pr.Total}
	} else if n > 0 {
		select {
		case pr.ProgressChan <- oci.Update{Complete: pr.Total}:
		default: // if the progress channel is full, it skips sending rather than blocking the Read() call.
		}
	}
	return n, err
}
