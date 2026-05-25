package ovms

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/model-runner/pkg/logging"
)

func TestBinaryPath(t *testing.T) {
	t.Run("uses custom binary path when provided", func(t *testing.T) {
		o := &ovms{customBinaryPath: "/tmp/custom-ovms"}
		if got := o.binaryPath(); got != "/tmp/custom-ovms" {
			t.Fatalf("binaryPath() = %q, want %q", got, "/tmp/custom-ovms")
		}
	})

	t.Run("uses ovms from PATH when custom path is empty", func(t *testing.T) {
		binDir := t.TempDir()
		binary := filepath.Join(binDir, Name)
		if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatalf("write fake ovms binary: %v", err)
		}

		originalPath := os.Getenv("PATH")
		t.Setenv("PATH", binDir+string(os.PathListSeparator)+originalPath)

		o := &ovms{}
		if got := o.binaryPath(); got != binary {
			t.Fatalf("binaryPath() = %q, want %q", got, binary)
		}
	})
}

func TestResolveOVMSModelPath(t *testing.T) {
	t.Run("uses model subdirectory when present", func(t *testing.T) {
		bundleRoot := t.TempDir()
		modelDir := filepath.Join(bundleRoot, "model")
		if err := os.MkdirAll(modelDir, 0755); err != nil {
			t.Fatalf("mkdir model dir: %v", err)
		}

		got := resolveOVMSModelPath(bundleRoot)
		if got != modelDir {
			t.Fatalf("resolveOVMSModelPath() = %q, want %q", got, modelDir)
		}
	})

	t.Run("falls back to bundle root when model subdirectory is missing", func(t *testing.T) {
		bundleRoot := t.TempDir()
		got := resolveOVMSModelPath(bundleRoot)
		if got != bundleRoot {
			t.Fatalf("resolveOVMSModelPath() = %q, want %q", got, bundleRoot)
		}
	})
}

func TestOVMSLogLevel(t *testing.T) {
	t.Run("debug logger uses DEBUG", func(t *testing.T) {
		logger := logging.NewLogger(slog.LevelDebug)
		if got := ovmsLogLevel(logger); got != "DEBUG" {
			t.Fatalf("ovmsLogLevel() = %q, want %q", got, "DEBUG")
		}
	})

	t.Run("non-debug logger uses INFO", func(t *testing.T) {
		logger := logging.NewLogger(slog.LevelInfo)
		if got := ovmsLogLevel(logger); got != "INFO" {
			t.Fatalf("ovmsLogLevel() = %q, want %q", got, "INFO")
		}
	})
}
