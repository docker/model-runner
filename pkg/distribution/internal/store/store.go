package store

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/docker/model-runner/pkg/distribution/internal/progress"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/oci/remote"
)

const (
	// CurrentVersion is the current version of the store layout
	CurrentVersion = "1.0.0"
)

// LocalStore implements the Store interface for local storage
type LocalStore struct {
	rootPath string
}

// RootPath returns the root path of the store
func (s *LocalStore) RootPath() string {
	return s.rootPath
}

// Options represents options for creating a store
type Options struct {
	RootPath string
}

// New creates a new LocalStore
func New(opts Options) (*LocalStore, error) {
	store := &LocalStore{
		rootPath: opts.RootPath,
	}

	// Initialize store if it doesn't exist
	if err := store.initialize(); err != nil {
		return nil, fmt.Errorf("initializing store: %w", err)
	}

	return store, nil
}

// Reset clears all contents of the store directory and reinitializes the store.
// It removes all files and subdirectories within the store's root path, but preserves the root directory itself.
// This allows the method to work correctly when the store directory is a mounted volume (e.g., in Docker Engine).
func (s *LocalStore) Reset() error {
	entries, err := os.ReadDir(s.rootPath)
	if err != nil {
		return fmt.Errorf("reading store directory: %w", err)
	}

	for _, entry := range entries {
		entryPath := filepath.Join(s.rootPath, entry.Name())
		if err := os.RemoveAll(entryPath); err != nil {
			return fmt.Errorf("removing %s: %w", entryPath, err)
		}
	}

	return s.initialize()
}

// initialize creates the store directory structure if it doesn't exist
func (s *LocalStore) initialize() error {
	// Check if layout.json exists, create if not
	if err := s.ensureLayout(); err != nil {
		return err
	}

	// Check if models.json exists, create if not
	if _, err := os.Stat(s.indexPath()); os.IsNotExist(err) {
		if err := s.writeIndex(Index{
			Models: []IndexEntry{},
		}); err != nil {
			return fmt.Errorf("initializing index file: %w", err)
		}
	}

	// Clean up stale incomplete files (older than 7 days)
	// This prevents disk space leaks from abandoned downloads
	if err := s.CleanupStaleIncompleteFiles(7 * 24 * time.Hour); err != nil {
		// Log the error but don't fail initialization
		fmt.Printf("Warning: failed to clean up stale incomplete files: %v\n", err)
	}

	return nil
}

// List lists all models in the store
func (s *LocalStore) List() ([]IndexEntry, error) {
	index, err := s.readIndex()
	if err != nil {
		return nil, fmt.Errorf("reading models index: %w", err)
	}
	return index.Models, nil
}

// Delete deletes a model by reference
func (s *LocalStore) Delete(ref string) (string, []string, error) {
	idx, err := s.readIndex()
	if err != nil {
		return "", nil, fmt.Errorf("reading models file: %w", err)
	}
	model, _, ok := idx.Find(ref)
	if !ok {
		return "", nil, ErrModelNotFound
	}

	digest, err := oci.NewHash(model.ID)
	if err != nil {
		return "", nil, fmt.Errorf("parse manifest digest %q: %w", model.ID, err)
	}

	// Remove manifest file
	if err := s.removeManifest(digest); err != nil {
		fmt.Printf("Warning: failed to remove manifest %q: %v\n", digest, err)
	}

	// Remove bundle if one exists
	if err := s.removeBundle(digest); err != nil {
		fmt.Printf("Warning: failed to remove bundle %q: %v\n", digest, err)
	}

	// Before deleting blobs, check if they are referenced by other models
	blobRefs := make(map[string]int)
	for _, m := range idx.Models {
		if m.ID == model.ID {
			continue // Skip the model being deleted
		}
		for _, file := range m.Files {
			blobRefs[file]++
		}
	}
	// Only delete blobs that are not referenced by other models
	for _, blobFile := range model.Files {
		if blobRefs[blobFile] > 0 {
			// Skip deletion if blob is referenced by other models
			continue
		}
		hash, err := oci.NewHash(blobFile)
		if err != nil {
			fmt.Printf("Warning: failed to parse blob hash %s: %v\n", blobFile, err)
			continue
		}
		if err := s.removeBlob(hash); err != nil {
			// Just log the error but don't fail the operation
			fmt.Printf("Warning: failed to remove blob %q from store: %v\n", hash.String(), err)
		}
	}

	idx = idx.Remove(model.ID)

	return model.ID, model.Tags, s.writeIndex(idx)
}

// AddTags adds tags to an existing model
func (s *LocalStore) AddTags(ref string, newTags []string) error {
	index, err := s.readIndex()
	if err != nil {
		return fmt.Errorf("reading models file: %w", err)
	}
	for _, t := range newTags {
		index, err = index.Tag(ref, t)
		if err != nil {
			return fmt.Errorf("tagging model: %w", err)
		}
	}

	return s.writeIndex(index)
}

// RemoveTags removes tags from models
func (s *LocalStore) RemoveTags(tags []string) ([]string, error) {
	index, err := s.readIndex()
	if err != nil {
		return nil, fmt.Errorf("reading modelss index: %w", err)
	}
	var tagRefs []string
	for _, tag := range tags {
		tagRef, newIndex, err := index.UnTag(tag)
		if err != nil {
			// Try to save progress before returning error.
			if writeIndexErr := s.writeIndex(newIndex); writeIndexErr != nil {
				return tagRefs, fmt.Errorf("untagging model: %w, also failed to save: %w", err, writeIndexErr)
			}
			return tagRefs, fmt.Errorf("untagging model: %w", err)
		}
		tagRefs = append(tagRefs, tagRef.String())
		index = newIndex
	}
	return tagRefs, s.writeIndex(index)
}

// Version returns the store version
func (s *LocalStore) Version() string {
	layout, err := s.readLayout()
	if err != nil {
		return "unknown"
	}

	return layout.Version
}

// syncWriter is a thread-safe wrapper around io.Writer for concurrent writes
type syncWriter struct {
	w  io.Writer
	mu sync.Mutex
}

// Write implements io.Writer interface with mutex protection
func (sw *syncWriter) Write(p []byte) (n int, err error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.w.Write(p)
}

// WriteOption configures Write behavior.
type WriteOption func(*writeOptions)

type writeOptions struct {
	rangeSuccess *remote.RangeSuccess
}

// WithRangeSuccess passes a RangeSuccess tracker for resume detection.
func WithRangeSuccess(rs *remote.RangeSuccess) WriteOption {
	return func(o *writeOptions) {
		o.rangeSuccess = rs
	}
}

// Write writes a model to the store
func (s *LocalStore) Write(mdl oci.Image, tags []string, w io.Writer, opts ...WriteOption) (err error) {
	var options writeOptions
	for _, opt := range opts {
		opt(&options)
	}
	initialIndex, err := s.readIndex()
	if err != nil {
		return fmt.Errorf("reading models index: %w", err)
	}

	type cleanupFunc func() error
	var cleanups []cleanupFunc
	success := false
	var rollbackErrors []error
	defer func() {
		if success {
			return
		}
		for i := len(cleanups) - 1; i >= 0; i-- {
			if cleanupErr := cleanups[i](); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
				rollbackErrors = append(rollbackErrors, cleanupErr)
			}
		}
		if len(rollbackErrors) > 0 {
			joined := errors.Join(rollbackErrors...)
			wrapped := fmt.Errorf("rollback cleanup errors: %w", joined)
			if err != nil {
				err = errors.Join(err, wrapped)
			} else {
				err = wrapped
			}
		}
	}()

	configCreated, err := s.writeConfigFile(mdl)
	if err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}
	if configCreated {
		cfgHash, hashErr := mdl.ConfigName()
		if hashErr != nil {
			return fmt.Errorf("config digest: %w", hashErr)
		}
		cleanups = append(cleanups, func() error {
			if err := s.removeBlob(cfgHash); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove config blob: %w", err)
			}
			return nil
		})
	}

	layers, err := mdl.Layers()
	if err != nil {
		return fmt.Errorf("getting layers: %w", err)
	}

	imageSize := int64(0)
	for _, layer := range layers {
		size, err := layer.Size()
		if err != nil {
			return fmt.Errorf("getting layer size: %w", err)
		}
		imageSize += size
	}

	// Create a thread-safe writer wrapper for concurrent progress reporting
	var safeWriter io.Writer
	if w != nil {
		safeWriter = &syncWriter{w: w}
	}

	// Pull all layers in parallel
	type layerResult struct {
		created bool
		diffID  oci.Hash
		err     error
	}

	results := make([]layerResult, len(layers))
	var wg sync.WaitGroup

	for i, layer := range layers {
		wg.Add(1)
		go func(idx int, l oci.Layer) {
			defer wg.Done()

			var pr *progress.Reporter
			var progressChan chan<- oci.Update
			if safeWriter != nil {
				pr = progress.NewProgressReporter(safeWriter, progress.PullMsg, imageSize, l, "pull")
				progressChan = pr.Updates()
			}

			created, diffID, err := s.writeLayer(l, progressChan, options.rangeSuccess)

			if progressChan != nil {
				close(progressChan)
				if pr != nil {
					if waitErr := pr.Wait(); waitErr != nil {
						fmt.Printf("reporter finished with non-fatal error: %v\n", waitErr)
					}
				}
			}

			results[idx] = layerResult{
				created: created,
				diffID:  diffID,
				err:     err,
			}
		}(i, layer)
	}

	// Wait for all layers to complete
	wg.Wait()

	// Check for errors and collect them all
	var allErrors []error
	for _, result := range results {
		if result.err != nil {
			allErrors = append(allErrors, fmt.Errorf("writing blob: %w", result.err))
		}
	}
	if err := errors.Join(allErrors...); err != nil {
		return err
	}

	// Collect new layer digests
	var newLayerDigests []oci.Hash
	for _, result := range results {
		if result.created {
			newLayerDigests = append(newLayerDigests, result.diffID)
		}
	}

	if len(newLayerDigests) > 0 {
		digests := append([]oci.Hash(nil), newLayerDigests...)
		cleanups = append(cleanups, func() error {
			var errs []error
			for _, dg := range digests {
				if err := s.removeBlob(dg); err != nil && !errors.Is(err, os.ErrNotExist) {
					errs = append(errs, fmt.Errorf("remove blob %s: %w", dg, err))
				}
			}
			if len(errs) > 0 {
				return errors.Join(errs...)
			}
			return nil
		})
	}

	// Write the manifest
	digest, err := mdl.Digest()
	if err != nil {
		return fmt.Errorf("get digest: %w", err)
	}
	rm, err := mdl.RawManifest()
	if err != nil {
		return fmt.Errorf("get raw manifest: %w", err)
	}
	manifestExists := false
	if _, statErr := os.Stat(s.manifestPath(digest)); statErr == nil {
		manifestExists = true
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("stat manifest: %w", statErr)
	}
	if err := s.WriteManifest(digest, rm); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	if !manifestExists {
		cleanups = append(cleanups, func() error {
			if err := s.removeManifest(digest); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove manifest: %w", err)
			}
			return nil
		})
	}
	cleanups = append(cleanups, func() error {
		if err := s.writeIndex(initialIndex); err != nil {
			return fmt.Errorf("restore models index: %w", err)
		}
		return nil
	})
	if err := s.AddTags(digest.String(), tags); err != nil {
		return fmt.Errorf("adding tags: %w", err)
	}
	success = true
	return nil
}

// WriteLightweight writes only the manifest and config for a model, assuming layers already exist in the store.
// This is used for config-only modifications where the layer data hasn't changed.
func (s *LocalStore) WriteLightweight(mdl oci.Image, tags []string) (err error) {
	initialIndex, err := s.readIndex()
	if err != nil {
		return fmt.Errorf("reading models index: %w", err)
	}

	type cleanupFunc func() error
	var cleanups []cleanupFunc
	success := false
	var rollbackErrors []error
	defer func() {
		if success {
			return
		}
		for i := len(cleanups) - 1; i >= 0; i-- {
			if cleanupErr := cleanups[i](); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
				rollbackErrors = append(rollbackErrors, cleanupErr)
			}
		}
		if len(rollbackErrors) > 0 {
			joined := errors.Join(rollbackErrors...)
			wrapped := fmt.Errorf("rollback cleanup errors: %w", joined)
			if err != nil {
				err = errors.Join(err, wrapped)
			} else {
				err = wrapped
			}
		}
	}()

	// Verify that all layers already exist in the store
	layers, err := mdl.Layers()
	if err != nil {
		return fmt.Errorf("getting layers: %w", err)
	}

	for _, layer := range layers {
		digest, err := layer.Digest()
		if err != nil {
			return fmt.Errorf("getting layer digest: %w", err)
		}
		hasBlob, err := s.hasBlob(digest)
		if err != nil {
			return fmt.Errorf("checking if layer %s exists: %w", digest, err)
		}
		if !hasBlob {
			return fmt.Errorf("layer %s not found in store, cannot use lightweight write", digest)
		}
	}

	// Write the config JSON file
	configCreated, err := s.writeConfigFile(mdl)
	if err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}
	if configCreated {
		cfgHash, hashErr := mdl.ConfigName()
		if hashErr != nil {
			return fmt.Errorf("config digest: %w", hashErr)
		}
		cleanups = append(cleanups, func() error {
			if err := s.removeBlob(cfgHash); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove config blob: %w", err)
			}
			return nil
		})
	}

	// Write the manifest
	digest, err := mdl.Digest()
	if err != nil {
		return fmt.Errorf("get digest: %w", err)
	}
	rm, err := mdl.RawManifest()
	if err != nil {
		return fmt.Errorf("get raw manifest: %w", err)
	}
	manifestExists := false
	if _, statErr := os.Stat(s.manifestPath(digest)); statErr == nil {
		manifestExists = true
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("stat manifest: %w", statErr)
	}
	if err := s.WriteManifest(digest, rm); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	if !manifestExists {
		cleanups = append(cleanups, func() error {
			if err := s.removeManifest(digest); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove manifest: %w", err)
			}
			return nil
		})
	}
	cleanups = append(cleanups, func() error {
		if err := s.writeIndex(initialIndex); err != nil {
			return fmt.Errorf("restore models index: %w", err)
		}
		return nil
	})
	if err := s.AddTags(digest.String(), tags); err != nil {
		return fmt.Errorf("adding tags: %w", err)
	}
	success = true
	return nil
}

// MigrateTags applies a transformation function to all tags in the index.
// If the function returns a different string, the tag is updated.
// Returns the number of tags that were migrated.
func (s *LocalStore) MigrateTags(transform func(string) string) (int, error) {
	index, err := s.readIndex()
	if err != nil {
		return 0, fmt.Errorf("reading index for migration: %w", err)
	}

	migrated := 0
	changed := false
	for i, entry := range index.Models {
		newTags := make([]string, len(entry.Tags))
		for j, tag := range entry.Tags {
			newTag := transform(tag)
			if newTag != tag {
				migrated++
				changed = true
			}
			newTags[j] = newTag
		}
		index.Models[i].Tags = newTags
	}

	if changed {
		if err := s.writeIndex(index); err != nil {
			return 0, fmt.Errorf("writing migrated index: %w", err)
		}
	}

	return migrated, nil
}

// Read reads a model from the store by reference (either tag or ID)
func (s *LocalStore) Read(reference string) (*Model, error) {
	models, err := s.List()
	if err != nil {
		return nil, fmt.Errorf("reading models file: %w", err)
	}

	// Find the model by tag
	for _, model := range models {
		if model.MatchesReference(reference) {
			hash, err := oci.NewHash(model.ID)
			if err != nil {
				return nil, fmt.Errorf("parsing hash: %w", err)
			}
			return s.newModel(hash, model.Tags)
		}
	}

	return nil, ErrModelNotFound
}
