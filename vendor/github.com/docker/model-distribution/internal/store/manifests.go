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
		if !s.hasBlob(layer.Digest) {
			return errors.New("missing blob %q for manifest - refusing to write unless all blobs exist")
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

	return s.writeIndex(idx.Add(newEntryForManifest(hash, manifest)))
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
	if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
		return fmt.Errorf("create parent directory %q: %w", filepath.Dir(path), err)
	}
	return os.WriteFile(path, data, 0666)
}
