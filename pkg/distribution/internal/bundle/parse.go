package bundle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/model-runner/pkg/distribution/types"
)

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

	// Ensure at least one model weight format is present
	if ggufPath == "" && safetensorsPath == "" {
		return nil, fmt.Errorf("no supported model weights found (neither GGUF nor safetensors)")
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
		runtimeConfig:    cfg,
		chatTemplatePath: templatePath,
	}, nil
}

func parseRuntimeConfig(rootDir string) (types.Config, error) {
	f, err := os.Open(filepath.Join(rootDir, "config.json"))
	if err != nil {
		return types.Config{}, fmt.Errorf("open runtime config: %w", err)
	}
	defer f.Close()
	var cfg types.Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return types.Config{}, fmt.Errorf("decode runtime config: %w", err)
	}
	return cfg, nil
}

func findGGUFFile(modelDir string) (string, error) {
	ggufs, err := filepath.Glob(filepath.Join(modelDir, "[^.]*.gguf"))
	if err != nil {
		return "", fmt.Errorf("find gguf files: %w", err)
	}
	if len(ggufs) == 0 {
		// GGUF files are optional - safetensors models won't have them
		return "", nil
	}
	return filepath.Base(ggufs[0]), nil
}

func findSafetensorsFile(modelDir string) (string, error) {
	safetensors, err := filepath.Glob(filepath.Join(modelDir, "[^.]*.safetensors"))
	if err != nil {
		return "", fmt.Errorf("find safetensors files: %w", err)
	}
	if len(safetensors) == 0 {
		// Safetensors files are optional - GGUF models won't have them
		return "", nil
	}
	return filepath.Base(safetensors[0]), nil
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
