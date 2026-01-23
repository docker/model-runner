package tarball

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/docker/model-runner/pkg/distribution/internal/progress"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
)

// Target stores an artifact as a TAR archive
type Target struct {
	writer io.Writer
	dirs   map[string]struct{}
}

// NewTarget returns a *Target for the given writer
func NewTarget(w io.Writer) (*Target, error) {
	return &Target{
		writer: w,
		dirs:   make(map[string]struct{}),
	}, nil
}

// Write writes the artifact in archive format to the configured io.Writer
func (t *Target) Write(ctx context.Context, mdl types.ModelArtifact, progressWriter io.Writer) error {
	tw := tar.NewWriter(t.writer)
	defer tw.Close()

	rm, err := mdl.RawManifest()
	if err != nil {
		return err
	}

	if err := t.ensureDir("blobs", tw); err != nil {
		return err
	}

	ls, err := mdl.Layers()
	if err != nil {
		return fmt.Errorf("get layers: %w", err)
	}

	layersSize := int64(0)
	for _, layer := range ls {
		size, err := layer.Size()
		if err != nil {
			return fmt.Errorf("get layer size: %w", err)
		}
		layersSize += size
	}

	for _, layer := range ls {
		if err := t.addLayer(layer, tw, progressWriter, layersSize); err != nil {
			return fmt.Errorf("add layer entry: %w", err)
		}
	}
	rcf, err := mdl.RawConfigFile()
	if err != nil {
		return err
	}
	cn, err := mdl.ConfigName()
	if err != nil {
		return err
	}
	if err = tw.WriteHeader(&tar.Header{
		Name: filepath.Join("blobs", cn.Algorithm, cn.Hex),
		Mode: 0666,
		Size: int64(len(rcf)),
	}); err != nil {
		return err
	}
	if _, err = tw.Write(rcf); err != nil {
		return fmt.Errorf("write config blob contents: %w", err)
	}

	if err := tw.WriteHeader(&tar.Header{
		Name: "manifest.json",
		Size: int64(len(rm)),
		Mode: 0666,
	}); err != nil {
		return fmt.Errorf("write manifest.json header: %w", err)
	}
	if _, err = tw.Write(rm); err != nil {
		return fmt.Errorf("write manifest.json contents: %w", err)
	}

	return nil
}

func (t *Target) addLayer(layer oci.Layer, tw *tar.Writer, progressWriter io.Writer, imageSize int64) error {
	diffID, err := layer.DiffID()
	if err != nil {
		return fmt.Errorf("get layer diffID: %w", err)
	}
	if err := t.ensureDir(filepath.Join("blobs", diffID.Algorithm), tw); err != nil {
		return err
	}
	sz, err := layer.Size()
	if err != nil {
		return fmt.Errorf("get layer size: %w", err)
	}
	if err = tw.WriteHeader(&tar.Header{
		Name: filepath.Join("blobs", diffID.Algorithm, diffID.Hex),
		Mode: 0666,
		Size: sz,
	}); err != nil {
		return fmt.Errorf("write blob file header: %w", err)
	}

	var pr *progress.Reporter
	var progressChan chan<- oci.Update
	if progressWriter != nil {
		pr = progress.NewProgressReporter(progressWriter, func(update oci.Update) string {
			return fmt.Sprintf("Transferred: %.2f MB", float64(update.Complete)/1024/1024)
		}, imageSize, layer, "pull")
		progressChan = pr.Updates()
		defer func() {
			close(progressChan)
			if err := pr.Wait(); err != nil {
				fmt.Printf("reporter finished with non-fatal error: %v\n", err)
			}
		}()
	}

	rc, err := layer.Uncompressed()
	if err != nil {
		return fmt.Errorf("open layer %q: %w", diffID, err)
	}
	defer rc.Close()
	if _, err = io.Copy(tw, progress.NewReader(rc, progressChan)); err != nil {
		return fmt.Errorf("copy layer %q: %w", diffID, err)
	}
	return nil
}

func (t *Target) ensureDir(path string, tw *tar.Writer) error {
	if _, ok := t.dirs[path]; !ok {
		if err := tw.WriteHeader(&tar.Header{
			Name:     path,
			Typeflag: tar.TypeDir,
		}); err != nil {
			return fmt.Errorf("add dir entry %q: %w", path, err)
		}
	}
	t.dirs[path] = struct{}{}
	return nil
}
