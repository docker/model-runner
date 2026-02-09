package gpu

import (
	"context"
	"os/exec"

	"github.com/docker/docker/client"
)

// GPUSupport encodes the GPU support available on a Docker engine.
type GPUSupport uint8

const (
	// GPUSupportNone indicates no detectable GPU support.
	GPUSupportNone GPUSupport = iota
	// GPUSupportCUDA indicates CUDA GPU support.
	GPUSupportCUDA
	// GPUSupportROCm indicates ROCm GPU support.
	GPUSupportROCm
	// GPUSupportMUSA indicates MUSA GPU support.
	GPUSupportMUSA
	// GPUSupportCANN indicates Ascend NPU support.
	GPUSupportCANN
)

// ProbeGPUSupport determines whether or not the Docker engine has GPU support.
func ProbeGPUSupport(ctx context.Context, dockerClient client.SystemAPIClient) (GPUSupport, error) {
	// Query Docker Engine for its effective configuration.
	// Docker Info is the source of truth for which runtimes are actually usable.
	info, err := dockerClient.Info(ctx)
	if err != nil {
		// Preserve best-effort behavior: if Docker Info is unavailable (e.g. in
		// restricted or degraded environments), do not treat this as a hard failure.
		// Instead, assume no GPU support and allow callers to continue.
		return GPUSupportNone, nil
	}

	// Runtimes are checked in priority order, from highest to lowest.
	// The first matching runtime determines the selected GPU support.
	supportedRuntimes := []struct {
		name    string
		support GPUSupport
	}{
		{"nvidia", GPUSupportCUDA},   // 1. CUDA (NVIDIA)
		{"rocm", GPUSupportROCm},     // 2. ROCm (AMD)
		{"mthreads", GPUSupportMUSA}, // 3. MUSA (MThreads)
		{"cann", GPUSupportCANN},     // 4. Ascend CANN (Huawei)
	}

	for _, r := range supportedRuntimes {
		if _, ok := info.Runtimes[r.name]; ok {
			return r.support, nil
		}
	}

	// Legacy fallback
	// Older Docker setups may not register the NVIDIA runtime explicitly,
	// but still have the legacy nvidia-container-runtime available on PATH.
	if _, err := exec.LookPath("nvidia-container-runtime"); err == nil {
		return GPUSupportCUDA, nil
	}

	// No known GPU runtime detected.
	return GPUSupportNone, nil
}

// HasNVIDIARuntime determines whether there is an nvidia runtime available
func HasNVIDIARuntime(ctx context.Context, dockerClient client.SystemAPIClient) (bool, error) {
	info, err := dockerClient.Info(ctx)
	if err != nil {
		return false, err
	}
	_, hasNvidia := info.Runtimes["nvidia"]
	return hasNvidia, nil
}

// HasROCmRuntime determines whether there is a ROCm runtime available
func HasROCmRuntime(ctx context.Context, dockerClient client.SystemAPIClient) (bool, error) {
	info, err := dockerClient.Info(ctx)
	if err != nil {
		return false, err
	}
	_, hasROCm := info.Runtimes["rocm"]
	return hasROCm, nil
}

// HasMTHREADSRuntime determines whether there is a mthreads runtime available
func HasMTHREADSRuntime(ctx context.Context, dockerClient client.SystemAPIClient) (bool, error) {
	info, err := dockerClient.Info(ctx)
	if err != nil {
		return false, err
	}
	_, hasMTHREADS := info.Runtimes["mthreads"]
	return hasMTHREADS, nil
}

// HasCANNRuntime determines whether there is a Ascend CANN runtime available
func HasCANNRuntime(ctx context.Context, dockerClient client.SystemAPIClient) (bool, error) {
	info, err := dockerClient.Info(ctx)
	if err != nil {
		return false, err
	}
	_, hasCANN := info.Runtimes["cann"]
	return hasCANN, nil
}
