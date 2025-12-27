package store

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/model-runner/pkg/distribution/internal/bundle"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
)

const (
	bundlesDir = "bundles"
)

// bundlePath returns the path to the bundle directory for the given hash.
func (s *LocalStore) bundlePath(hash oci.Hash) string {
	return filepath.Join(s.rootPath, bundlesDir, hash.Algorithm, hash.Hex)
}

// BundleForModel returns a runtime bundle for the given model
func (s *LocalStore) BundleForModel(ref string) (types.ModelBundle, error) {
	mdl, err := s.Read(ref)
	if err != nil {
		return nil, fmt.Errorf("find model content: %w", err)
	}
	dgst, err := mdl.Digest()
	if err != nil {
		return nil, fmt.Errorf("get model ID: %w", err)
	}
	path := s.bundlePath(dgst)
	if bdl, err := bundle.Parse(path); err != nil {
		// create for first time or replace bad/corrupted bundle
		return s.createBundle(path, mdl)
	} else {
		return bdl, nil
	}
}

// createBundle unpacks the bundle to path, replacing existing bundle if one is found
func (s *LocalStore) createBundle(path string, mdl *Model) (types.ModelBundle, error) {
	if err := os.RemoveAll(path); err != nil {
		return nil, fmt.Errorf("remove %s: %w", path, err)
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("create bundle directory: %w", err)
	}
	bdl, err := bundle.Unpack(path, mdl)
	if err != nil {
		return nil, fmt.Errorf("unpack bundle: %w", err)
	}
	return bdl, nil
}

func (s *LocalStore) removeBundle(hash oci.Hash) error {
	return os.RemoveAll(s.bundlePath(hash))
}
