package store

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/docker/model-distribution/internal/progress"
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
// This allows the method to work correctly when the store directory is a mounted volume (e.g., in Docker CE).
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

	// Remove manifest file
	if digest, err := v1.NewHash(model.ID); err != nil {
		fmt.Printf("Warning: failed to parse manifest digest %s: %v\n", digest, err)
	} else if err := s.removeManifest(digest); err != nil {
		fmt.Printf("Warning: failed to remove manifest %q: %v\n",
			digest, err,
		)
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
		hash, err := v1.NewHash(blobFile)
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
		tagRefs = append(tagRefs, tagRef.Name())
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

// Write writes a model to the store
func (s *LocalStore) Write(mdl v1.Image, tags []string, w io.Writer) error {
	// Write the config JSON file
	if err := s.writeConfigFile(mdl); err != nil {
		return fmt.Errorf("writing config file: %w", err)
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

	for _, layer := range layers {
		var pr *progress.Reporter
		var progressChan chan<- v1.Update
		if w != nil {
			pr = progress.NewProgressReporter(w, progress.PullMsg, imageSize, layer)
			progressChan = pr.Updates()
		}

		err := s.writeLayer(layer, progressChan)

		if progressChan != nil {
			close(progressChan)
			if err := pr.Wait(); err != nil {
				fmt.Printf("reporter finished with non-fatal error: %v\n", err)
			}
		}

		if err != nil {
			return fmt.Errorf("writing blob: %w", err)
		}
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
	if err := s.WriteManifest(digest, rm); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	if err := s.AddTags(digest.String(), tags); err != nil {
		return fmt.Errorf("adding tags: %w", err)
	}
	return err
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
			hash, err := v1.NewHash(model.ID)
			if err != nil {
				return nil, fmt.Errorf("parsing hash: %w", err)
			}
			return s.newModel(hash, model.Tags)
		}
	}

	return nil, ErrModelNotFound
}
