//go:build !linux

package gpuinfo

func (g *GPUInfo) HasSupportedAMDGPU() (bool, error) {
	// AMD GPU detection is only supported on Linux
	return false, nil
}