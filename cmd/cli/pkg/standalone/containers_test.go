package standalone

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/model-runner/cmd/cli/pkg/types"
)

// TestSyncDockerConfigToContainer_NoopForDesktopAndCloud verifies that
// SyncDockerConfigToContainer skips Desktop and Cloud engine kinds.
func TestSyncDockerConfigToContainer_NoopForDesktopAndCloud(t *testing.T) {
	tmpDir := t.TempDir()
	dockerDir := filepath.Join(tmpDir, ".docker")
	if err := os.MkdirAll(dockerDir, 0o700); err != nil {
		t.Fatalf("failed to create .docker dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dockerDir, "config.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatalf("failed to write config.json: %v", err)
	}
	t.Setenv("HOME", tmpDir)

	for _, engineKind := range []types.ModelRunnerEngineKind{
		types.ModelRunnerEngineKindDesktop,
		types.ModelRunnerEngineKindCloud,
	} {
		t.Run(engineKind.String(), func(t *testing.T) {
			err := SyncDockerConfigToContainer(t.Context(), nil, "container-id", engineKind)
			if err != nil {
				t.Fatalf("SyncDockerConfigToContainer(%v) returned unexpected error: %v", engineKind, err)
			}
		})
	}
}

// TestSyncDockerConfigToContainer_NoopWhenConfigMissing verifies that
// SyncDockerConfigToContainer skips when the host config file is absent.
func TestSyncDockerConfigToContainer_NoopWhenConfigMissing(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	err := SyncDockerConfigToContainer(t.Context(), nil, "container-id", types.ModelRunnerEngineKindMoby)
	if err != nil {
		t.Fatalf("SyncDockerConfigToContainer returned unexpected error for missing config: %v", err)
	}
}
