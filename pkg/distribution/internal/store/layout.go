package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Layout represents the layout information of the store
type Layout struct {
	Version string `json:"version"`
}

// layoutPath returns the path to the layout file
func (s *LocalStore) layoutPath() string {
	return filepath.Join(s.rootPath, "layout.json")
}

// readLayout reads the layout file and returns the layout information
func (s *LocalStore) readLayout() (Layout, error) {
	// Version returns the store version
	// Read the layout file
	layoutData, err := os.ReadFile(s.layoutPath())
	if err != nil {
		return Layout{}, fmt.Errorf("read layout path path %q: %w", s.layoutPath(), err)
	}

	// Unmarshal the layout
	var layout Layout
	if err := json.Unmarshal(layoutData, &layout); err != nil {
		return Layout{}, fmt.Errorf("unmarshal layout: %w", err)
	}

	return layout, nil
}

// ensureLayout ensure a layout file exists
func (s *LocalStore) ensureLayout() error {
	if _, err := os.Stat(s.layoutPath()); os.IsNotExist(err) {
		layout := Layout{
			Version: CurrentVersion,
		}
		if err := s.writeLayout(layout); err != nil {
			return fmt.Errorf("initializing layout file: %w", err)
		}
	}
	return nil
}

// writeLayout write the layout file
func (s *LocalStore) writeLayout(layout Layout) error {
	layoutData, err := json.MarshalIndent(layout, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling layout: %w", err)
	}
	if err := writeFile(s.layoutPath(), layoutData); err != nil {
		return fmt.Errorf("writing layout file: %w", err)
	}
	return nil
}
