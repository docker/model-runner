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
