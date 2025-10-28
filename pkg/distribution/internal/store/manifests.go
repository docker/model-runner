package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/v1"
)

const (
	manifestsDir = "manifests"
)

// manifestPath returns the path to the manifest file for the given hash.
func (s *LocalStore) manifestPath(hash v1.Hash) string {
	return filepath.Join(s.rootPath, manifestsDir, hash.Algorithm, hash.Hex)
}

// WriteManifest writes the model's manifest to the store
func (s *LocalStore) WriteManifest(hash v1.Hash, raw []byte) error {
	manifest, err := v1.ParseManifest(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}
	for _, layer := range manifest.Layers {
		hasBlob, err := s.hasBlob(layer.Digest)
		if err != nil {
			return fmt.Errorf("check blob existence: %w", err)
		}
		if !hasBlob {
			return fmt.Errorf("missing blob %q for manifest - refusing to write unless all blobs exist", layer.Digest)
		}
	}
	if err := writeFile(s.manifestPath(hash), raw); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	// Add the manifest to the index
	idx, err := s.readIndex()
	if err != nil {
		return fmt.Errorf("reading models: %w", err)
	}

	if err := s.writeIndex(idx.Add(newEntryForManifest(hash, manifest))); err != nil {
		// Best effort rollback to avoid leaving an orphaned manifest on disk.
		if removeErr := s.removeManifest(hash); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return fmt.Errorf("writing models index: %w (also failed to remove manifest: %v)", err, removeErr)
		}
		return fmt.Errorf("writing models index: %w", err)
	}
	return nil
}

func newEntryForManifest(digest v1.Hash, manifest *v1.Manifest) IndexEntry {
	files := make([]string, len(manifest.Layers)+1)
	for i := range manifest.Layers {
		files[i] = manifest.Layers[i].Digest.String()
	}
	files[len(manifest.Layers)] = manifest.Config.Digest.String()

	return IndexEntry{
		ID:    digest.String(),
		Files: files,
	}
}

// removeManifest removes the manifest file from the store
func (s *LocalStore) removeManifest(hash v1.Hash) error {
	return os.Remove(s.manifestPath(hash))
}

// writeFile is a wrapper around os.WriteFile that creates any parent directories as needed.
func writeFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0777); err != nil {
		return fmt.Errorf("create parent directory %q: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = os.Remove(tmpName)
	}

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("write temporary file %q: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("sync temporary file %q: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temporary file %q: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, 0666); err != nil && !errors.Is(err, os.ErrPermission) {
		cleanup()
		return fmt.Errorf("chmod temporary file %q: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		removeErr := os.Remove(path)
		if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			cleanup()
			return fmt.Errorf("replace %q with temporary file: %w (also failed to remove existing file: %v)", path, err, removeErr)
		}
		if err := os.Rename(tmpName, path); err != nil {
			cleanup()
			return fmt.Errorf("replace %q with temporary file: %w", path, err)
		}
	}
	return nil
}
