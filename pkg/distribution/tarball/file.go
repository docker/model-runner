package tarball

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/docker/model-runner/pkg/distribution/types"
)

// FileTarget writes an artifact tarball to a local file.
type FileTarget struct {
	path string
}

// NewFileTarget returns a *FileTarget for the given path.
func NewFileTarget(path string) *FileTarget {
	return &FileTarget{
		path: path,
	}
}

// Write writes the given artifact to the target.
func (t *FileTarget) Write(ctx context.Context, mdl types.ModelArtifact, pw io.Writer) error {
	f, err := os.Create(t.path)
	if err != nil {
		return fmt.Errorf("create file for archive: %w", err)
	}
	defer f.Close()
	target, err := NewTarget(f)
	if err != nil {
		return fmt.Errorf("create target: %w", err)
	}
	return target.Write(ctx, mdl, pw)
}
