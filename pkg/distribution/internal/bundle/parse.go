package bundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/model-runner/pkg/distribution/modelpack"
	"github.com/docker/model-runner/pkg/distribution/types"
)

// errFoundModelFile is a sentinel error used to stop filepath.Walk early after
// finding the first matching model file.
var errFoundModelFile = fmt.Errorf("found model file")

// Parse returns the Bundle at the given rootDir
func Parse(rootDir string) (*Bundle, error) {
	if fi, err := os.Stat(rootDir); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("inspect bundle root dir: %w", err)
	}

	// Check if model subdirectory exists - required for new bundle format
	// If it doesn't exist, this is an old bundle format that needs to be recreated
	modelDir := filepath.Join(rootDir, ModelSubdir)
	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("bundle uses old format (missing %s subdirectory), will be recreated", ModelSubdir)
	}

	ggufPath, err := findGGUFFile(modelDir)
	if err != nil {
		return nil, err
	}
	safetensorsPath, err := findSafetensorsFile(modelDir)
	if err != nil {
		return nil, err
	}
	ddufPath, err := findDDUFFile(modelDir)
	if err != nil {
		return nil, err
	}

	// Ensure at least one model weight format is present
	if ggufPath == "" && safetensorsPath == "" && ddufPath == "" {
		return nil, fmt.Errorf("no supported model weights found (neither GGUF, safetensors, nor DDUF)")
	}

	mmprojPath, err := findMultiModalProjectorFile(modelDir)
	if err != nil {
		return nil, err
	}
	templatePath, err := findChatTemplateFile(modelDir)
	if err != nil {
		return nil, err
	}

	// Runtime config stays at bundle root
	cfg, err := parseRuntimeConfig(rootDir)
	if err != nil {
		return nil, err
	}
	return &Bundle{
		dir:              rootDir,
		mmprojPath:       mmprojPath,
		ggufFile:         ggufPath,
		safetensorsFile:  safetensorsPath,
		ddufFile:         ddufPath,
		runtimeConfig:    cfg,
		chatTemplatePath: templatePath,
	}, nil
}

// parseRuntimeConfig parses the runtime config from the bundle.
// Natively supports both Docker format and ModelPack format without conversion.
func parseRuntimeConfig(rootDir string) (types.ModelConfig, error) {
	raw, err := os.ReadFile(filepath.Join(rootDir, "config.json"))
	if err != nil {
		return nil, fmt.Errorf("read runtime config: %w", err)
	}

	// Detect and parse based on format
	if modelpack.IsModelPackConfig(raw) {
		var mp modelpack.Model
		if err := json.Unmarshal(raw, &mp); err != nil {
			return nil, fmt.Errorf("decode ModelPack runtime config: %w", err)
		}
		return &mp, nil
	}

	// Docker format
	var cfg types.Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("decode Docker runtime config: %w", err)
	}
	return &cfg, nil
}

// findModelFile finds a supported model file by extension. It prefers a
// top-level match in modelDir and falls back to a recursive search when needed.
// Hidden files are ignored.
func findModelFile(modelDir, ext string) (string, error) {
	pattern := filepath.Join(modelDir, "[^.]*"+ext)
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("find %s files: %w", ext, err)
	}
	if len(paths) > 0 {
		return filepath.Base(paths[0]), nil
	}

	var firstFound string
	walkErr := filepath.Walk(
		modelDir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				// Propagate filesystem errors so callers can distinguish them
				// from the case where no matching files are present.
				return err
			}
			if info.IsDir() {
				return nil
			}
			if filepath.Ext(path) != ext ||
				strings.HasPrefix(info.Name(), ".") {
				return nil
			}

			rel, relErr := filepath.Rel(modelDir, path)
			if relErr != nil {
				// Treat a bad relative path as a real error instead of
				// silently ignoring it, so malformed bundles surface to the
				// caller.
				return relErr
			}
			firstFound = rel
			return errFoundModelFile
		},
	)
	if walkErr != nil && !errors.Is(walkErr, errFoundModelFile) {
		return "", fmt.Errorf("walk for %s files: %w", ext, walkErr)
	}

	return firstFound, nil
}

func findGGUFFile(modelDir string) (string, error) {
	// GGUF files are optional.
	return findModelFile(modelDir, ".gguf")
}

func findSafetensorsFile(modelDir string) (string, error) {
	// Safetensors files are optional.
	return findModelFile(modelDir, ".safetensors")
}

func findDDUFFile(modelDir string) (string, error) {
	// DDUF files are optional.
	return findModelFile(modelDir, ".dduf")
}

func findMultiModalProjectorFile(modelDir string) (string, error) {
	mmprojPaths, err := filepath.Glob(filepath.Join(modelDir, "[^.]*.mmproj"))
	if err != nil {
		return "", err
	}
	if len(mmprojPaths) == 0 {
		return "", nil
	}
	if len(mmprojPaths) > 1 {
		return "", fmt.Errorf("found multiple .mmproj files, but only 1 is supported")
	}
	return filepath.Base(mmprojPaths[0]), nil
}

func findChatTemplateFile(modelDir string) (string, error) {
	templatePaths, err := filepath.Glob(filepath.Join(modelDir, "[^.]*.jinja"))
	if err != nil {
		return "", err
	}
	if len(templatePaths) == 0 {
		return "", nil
	}
	if len(templatePaths) > 1 {
		return "", fmt.Errorf("found multiple template files, but only 1 is supported")
	}
	return filepath.Base(templatePaths[0]), nil
}
