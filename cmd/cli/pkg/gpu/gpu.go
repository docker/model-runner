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
	// GPUSupportROCm indicates ROCm GPU support (AMD).
	GPUSupportROCm
)

// ProbeGPUSupport determines whether or not the Docker engine has GPU support.
func ProbeGPUSupport(ctx context.Context, dockerClient client.SystemAPIClient) (GPUSupport, error) {
	// First search for nvidia-container-runtime on PATH
	if _, err := exec.LookPath("nvidia-container-runtime"); err == nil {
		return GPUSupportCUDA, nil
	}

	// Next look for explicitly configured nvidia runtime. This is not required in Docker 19.03+ but
	// may be configured on some systems
	hasNvidia, err := HasNVIDIARuntime(ctx, dockerClient)
	if err != nil {
		return GPUSupportNone, err
	}
	if hasNvidia {
		return GPUSupportCUDA, nil
	}

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
