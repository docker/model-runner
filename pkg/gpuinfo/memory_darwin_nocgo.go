//go:build darwin && !cgo

package gpuinfo

import "errors"

// getVRAMSize returns total system GPU memory in bytes
func getVRAMSize(_ string) (uint64, error) {
	return 0, errors.New("unimplemented without cgo")
}

// hasSupportedAMDGPU returns true if the system has supported AMD GPUs
func hasSupportedAMDGPU() (bool, error) {
	// AMD GPU detection is only supported on Linux
	return false, nil
}
