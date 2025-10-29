//go:build darwin && cgo

package gpuinfo

/*
#cgo LDFLAGS: -framework Metal
#include "metal.h"
*/
import "C"
import "errors"

// getVRAMSize returns total system GPU memory in bytes
func getVRAMSize(_ string) (uint64, error) {
	vramSize := C.getVRAMSize()
	if vramSize == 0 {
		return 0, errors.New("could not get metal VRAM size")
	}
	return uint64(vramSize), nil
}

// hasSupportedAMDGPU returns true if the system has supported AMD GPUs
func hasSupportedAMDGPU() (bool, error) {
	// AMD GPU detection is only supported on Linux
	return false, nil
}

// hasSupportedMTHREADSGPU returns true if the system has supported AMD GPUs
func hasSupportedMTHREADSGPU() (bool, error) {
	// MTHREADS GPU detection is only supported on Linux
	return false, nil
}
