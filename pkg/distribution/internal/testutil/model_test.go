package testutil

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/model-runner/pkg/distribution/modelpack"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
)

func TestNewGGUFArtifactWithConfigMediaType(t *testing.T) {
	modelPath := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(modelPath, []byte("dummy gguf content"), 0644); err != nil {
		t.Fatalf("Failed to create GGUF fixture: %v", err)
	}

	artifact := NewGGUFArtifactWithConfigMediaType(
		t,
		modelPath,
		"application/vnd.docker.ai.model.config.v99.0+json",
	)

	id, err := artifact.ID()
	if err != nil {
		t.Fatalf("ID() error = %v", err)
	}
	if id == "" {
		t.Fatal("ID() returned empty string")
	}

	cfg, err := artifact.Config()
	if err != nil {
		t.Fatalf("Config() error = %v", err)
	}
	if cfg.GetFormat() != types.FormatGGUF {
		t.Fatalf("Config format = %q, want %q", cfg.GetFormat(), types.FormatGGUF)
	}

	manifest, err := artifact.Manifest()
	if err != nil {
		t.Fatalf("Manifest() error = %v", err)
	}
	if manifest.Config.MediaType != "application/vnd.docker.ai.model.config.v99.0+json" {
		t.Fatalf("Manifest config media type = %q", manifest.Config.MediaType)
	}
}

func TestNewModelPackArtifact(t *testing.T) {
	modelPath := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(modelPath, []byte("dummy modelpack content"), 0644); err != nil {
		t.Fatalf("Failed to create ModelPack fixture: %v", err)
	}

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	artifact := NewModelPackArtifact(t, modelpack.Model{
		Descriptor: modelpack.ModelDescriptor{
			CreatedAt: &now,
			Name:      "dummy-modelpack",
		},
		Config: modelpack.ModelConfig{
			Format:    "gguf",
			ParamSize: "8B",
		},
	}, Layer(modelPath, oci.MediaType(modelpack.MediaTypeWeightGGUF)))

	cfg, err := artifact.Config()
	if err != nil {
		t.Fatalf("Config() error = %v", err)
	}
	if _, ok := cfg.(*modelpack.Model); !ok {
		t.Fatalf("Config() type = %T, want *modelpack.Model", cfg)
	}
	if cfg.GetFormat() != types.FormatGGUF {
		t.Fatalf("Config format = %q, want %q", cfg.GetFormat(), types.FormatGGUF)
	}

	desc, err := artifact.Descriptor()
	if err != nil {
		t.Fatalf("Descriptor() error = %v", err)
	}
	if desc.Created == nil || !desc.Created.Equal(now) {
		t.Fatalf("Descriptor created = %v, want %v", desc.Created, now)
	}

	manifest, err := artifact.Manifest()
	if err != nil {
		t.Fatalf("Manifest() error = %v", err)
	}
	if manifest.Config.MediaType != oci.MediaType(modelpack.MediaTypeModelConfigV1) {
		t.Fatalf("Manifest config media type = %q, want %q", manifest.Config.MediaType, modelpack.MediaTypeModelConfigV1)
	}
}

func TestWithRawConfigError(t *testing.T) {
	modelPath := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(modelPath, []byte("dummy gguf content"), 0644); err != nil {
		t.Fatalf("Failed to create GGUF fixture: %v", err)
	}

	expectedErr := errors.New("forced config failure")
	artifact := WithRawConfigError(NewGGUFArtifact(t, modelPath), expectedErr)

	rcf, ok := artifact.(interface {
		RawConfigFile() ([]byte, error)
	})
	if !ok {
		t.Fatal("Artifact does not expose RawConfigFile")
	}

	if _, err := rcf.RawConfigFile(); !errors.Is(err, expectedErr) {
		t.Fatalf("RawConfigFile() error = %v, want %v", err, expectedErr)
	}
	if _, err := artifact.Config(); !errors.Is(err, expectedErr) {
		t.Fatalf("Config() error = %v, want %v", err, expectedErr)
	}
}

func TestWithLayersError(t *testing.T) {
	modelPath := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(modelPath, []byte("dummy gguf content"), 0644); err != nil {
		t.Fatalf("Failed to create GGUF fixture: %v", err)
	}

	expectedErr := errors.New("forced layers failure")
	artifact := WithLayersError(NewGGUFArtifact(t, modelPath), expectedErr)

	if _, err := artifact.Layers(); !errors.Is(err, expectedErr) {
		t.Fatalf("Layers() error = %v, want %v", err, expectedErr)
	}
}
