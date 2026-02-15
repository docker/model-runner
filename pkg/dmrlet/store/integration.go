// Package store provides integration with the Docker Model Runner model store.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultStorePath = "~/.docker/model-runner/models"
)

// Integration provides access to the Docker Model Runner model store.
type Integration struct {
	storePath string
}

// NewIntegration creates a new model store integration.
func NewIntegration(storePath string) *Integration {
	if storePath == "" {
		storePath = expandPath(defaultStorePath)
	}
	return &Integration{
		storePath: storePath,
	}
}

// ModelInfo holds information about a model in the store.
type ModelInfo struct {
	Name         string
	Tag          string
	Format       string // "gguf", "safetensors", etc.
	Size         int64
	Architecture string
	Parameters   string
	Quantization string
	Path         string
}

// GetModelPath returns the path to a model's files.
func (i *Integration) GetModelPath(modelRef string) (string, error) {
	// Parse model reference (e.g., "ai/llama3.2:latest", "llama3.2", etc.)
	name, tag := parseModelRef(modelRef)

	// Try different path patterns
	paths := i.getPossiblePaths(name, tag)

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("model %s not found in store at %s", modelRef, i.storePath)
}

// ListModels returns all models in the store.
func (i *Integration) ListModels() ([]ModelInfo, error) {
	var models []ModelInfo

	// Walk the store directory
	err := filepath.Walk(i.storePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if info.IsDir() {
			return nil
		}

		// Check for model files
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".gguf":
			models = append(models, ModelInfo{
				Name:   filepath.Base(filepath.Dir(path)),
				Format: "gguf",
				Size:   info.Size(),
				Path:   path,
			})
		case ".safetensors":
			models = append(models, ModelInfo{
				Name:   filepath.Base(filepath.Dir(path)),
				Format: "safetensors",
				Size:   info.Size(),
				Path:   path,
			})
		}

		return nil
	})

	return models, err
}

// GetModelInfo returns information about a specific model.
func (i *Integration) GetModelInfo(modelRef string) (*ModelInfo, error) {
	path, err := i.GetModelPath(modelRef)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	name, tag := parseModelRef(modelRef)
	format := detectFormat(path)

	model := &ModelInfo{
		Name:   name,
		Tag:    tag,
		Format: format,
		Size:   info.Size(),
		Path:   path,
	}

	// Try to read model metadata
	metaPath := filepath.Join(filepath.Dir(path), "config.json")
	if meta, err := i.readModelMeta(metaPath); err == nil {
		model.Architecture = meta.Architecture
		model.Parameters = meta.Parameters
		model.Quantization = meta.Quantization
	}

	return model, nil
}

// GetStorePath returns the base store path.
func (i *Integration) GetStorePath() string {
	return i.storePath
}

// ModelExists checks if a model exists in the store.
func (i *Integration) ModelExists(modelRef string) bool {
	_, err := i.GetModelPath(modelRef)
	return err == nil
}

func (i *Integration) getPossiblePaths(name, tag string) []string {
	var paths []string

	// Sanitize inputs to prevent path traversal
	safeName := sanitizePathComponent(name)
	safeTag := sanitizePathComponent(tag)

	// Pattern 1: Direct model file
	paths = append(paths, filepath.Join(i.storePath, safeName, safeTag, "model.gguf"))
	paths = append(paths, filepath.Join(i.storePath, safeName, safeTag, "model.safetensors"))

	// Pattern 2: Without tag directory
	paths = append(paths, filepath.Join(i.storePath, safeName, "model.gguf"))
	paths = append(paths, filepath.Join(i.storePath, safeName, "model.safetensors"))

	// Pattern 3: OCI blob store pattern
	// Models might be stored in blobs/<sha256>
	blobsDir := filepath.Join(i.storePath, "blobs")
	if entries, err := os.ReadDir(blobsDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				paths = append(paths, filepath.Join(blobsDir, entry.Name()))
			}
		}
	}

	// Pattern 4: With registry prefix (e.g., "ai/llama3.2")
	if strings.Contains(safeName, "/") {
		parts := strings.SplitN(safeName, "/", 2)
		// Sanitize individual parts as well
		part0 := sanitizePathComponent(parts[0])
		part1 := sanitizePathComponent(parts[1])
		paths = append(paths, filepath.Join(i.storePath, part0, part1, safeTag, "model.gguf"))
		paths = append(paths, filepath.Join(i.storePath, part0, part1, "model.gguf"))
	}

	// Pattern 5: Index lookup
	indexPath := filepath.Join(i.storePath, "index.json")
	if indexEntry, err := i.lookupInIndex(indexPath, safeName, safeTag); err == nil && indexEntry != "" {
		paths = append([]string{indexEntry}, paths...)
	}

	return paths
}

type indexEntry struct {
	Name   string `json:"name"`
	Tag    string `json:"tag"`
	Digest string `json:"digest"`
	Path   string `json:"path"`
}

type modelIndex struct {
	Entries []indexEntry `json:"entries"`
}

func (i *Integration) lookupInIndex(indexPath, name, tag string) (string, error) {
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return "", err
	}

	var index modelIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return "", err
	}

	for _, entry := range index.Entries {
		if entry.Name == name && (entry.Tag == tag || tag == "latest") {
			// Sanitize the path to prevent directory traversal
			safePath := sanitizePathComponent(entry.Path)

			// Construct the full path and ensure it's within the store directory
			fullPath := filepath.Join(i.storePath, safePath)
			absPath, err := filepath.Abs(fullPath)
			if err != nil {
				continue
			}

			absStorePath, err := filepath.Abs(i.storePath)
			if err != nil {
				continue
			}

			// Check if the resolved path is within the store directory
			if strings.HasPrefix(absPath, absStorePath+string(filepath.Separator)) || absPath == absStorePath {
				return fullPath, nil
			}
		}
	}

	return "", fmt.Errorf("not found in index")
}

type modelMeta struct {
	Architecture string `json:"architecture"`
	Parameters   string `json:"parameters"`
	Quantization string `json:"quantization"`
	Format       string `json:"format"`
}

func (i *Integration) readModelMeta(path string) (*modelMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var meta modelMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

func parseModelRef(ref string) (name, tag string) {
	// Remove leading registry if present
	ref = strings.TrimPrefix(ref, "docker.io/")
	ref = strings.TrimPrefix(ref, "library/")

	// Split name and tag
	parts := strings.SplitN(ref, ":", 2)
	name = parts[0]
	tag = "latest"
	if len(parts) > 1 {
		tag = parts[1]
	}

	return name, tag
}

func detectFormat(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".gguf":
		return "gguf"
	case ".safetensors":
		return "safetensors"
	case ".pt", ".pth":
		return "pytorch"
	case ".bin":
		return "binary"
	default:
		return "unknown"
	}
}

func sanitizePathComponent(component string) string {
	// Remove any path traversal sequences
	component = strings.ReplaceAll(component, "../", "")
	component = strings.ReplaceAll(component, "..\\", "")
	component = strings.ReplaceAll(component, "./", "")
	component = strings.ReplaceAll(component, ".\\", "")

	// Only allow alphanumeric characters, hyphens, underscores, and periods
	var sanitized strings.Builder
	for _, r := range component {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			sanitized.WriteRune(r)
		} else {
			// Replace invalid characters with underscore
			sanitized.WriteRune('_')
		}
	}

	result := sanitized.String()

	// Ensure the result is not empty or just dots
	if result == "" || result == "." || result == ".." {
		return "_"
	}

	return result
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
