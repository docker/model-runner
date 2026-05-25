package standalone

import (
	"testing"

	gpupkg "github.com/docker/model-runner/cmd/cli/pkg/gpu"
	"github.com/docker/model-runner/pkg/inference/backends/llamacpp"
	"github.com/docker/model-runner/pkg/inference/backends/vllm"
)

// TestControllerImageVariant_FallsBackToDefaultTag verifies that
// unsupported official Linux variants reuse the default image tag.
func TestControllerImageVariant_FallsBackToDefaultTag(t *testing.T) {
	testCases := []struct {
		name string
		gpu  gpupkg.GPUSupport
	}{
		{
			name: "musa",
			gpu:  gpupkg.GPUSupportMUSA,
		},
		{
			name: "cann",
			gpu:  gpupkg.GPUSupportCANN,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := controllerImageVariant(testCase.gpu, llamacpp.Name)
			if got != "" {
				t.Fatalf("controllerImageVariant() = %q, want empty", got)
			}
		})
	}
}

// TestControllerImageVariant_UsesVLLMTag verifies that the vLLM backend
// keeps its dedicated CUDA image tag.
func TestControllerImageVariant_UsesVLLMTag(t *testing.T) {
	got := controllerImageVariant(gpupkg.GPUSupportNone, vllm.Name)
	if got != "vllm-cuda" {
		t.Fatalf("controllerImageVariant() = %q, want %q", got, "vllm-cuda")
	}
}

// TestControllerImageVariant_RespectsEnvOverride verifies that an explicit
// controller image variant still overrides the default mapping.
func TestControllerImageVariant_RespectsEnvOverride(t *testing.T) {
	t.Setenv("MODEL_RUNNER_CONTROLLER_VARIANT", "musa")

	got := controllerImageVariant(gpupkg.GPUSupportNone, llamacpp.Name)
	if got != "musa" {
		t.Fatalf("controllerImageVariant() = %q, want %q", got, "musa")
	}
}
