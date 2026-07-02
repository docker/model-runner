package standalone

import (
	"testing"

	"github.com/docker/model-runner/cmd/cli/pkg/types"
)

// TestSyncDockerConfigToContainer_NoopForDesktopAndCloud verifies that
// SyncDockerConfigToContainer skips Desktop and Cloud engine kinds.
func TestSyncDockerConfigToContainer_NoopForDesktopAndCloud(t *testing.T) {
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
	t.Setenv("DOCKER_CONFIG", t.TempDir())

	err := SyncDockerConfigToContainer(t.Context(), nil, "container-id", types.ModelRunnerEngineKindMoby)
	if err != nil {
		t.Fatalf("SyncDockerConfigToContainer returned unexpected error for missing config: %v", err)
	}
}
