package scheduling

/*
#cgo LDFLAGS: -ldl
#include "nvidia.h"
*/
import "C"
import "errors"

// getVRAMSize returns total system GPU memory in bytes
func getVRAMSize() (uint64, error) {
	vramSize := C.getVRAMSize()
	if vramSize == 0 {
		return 0, errors.New("could not get nvidia VRAM size")
	}
	return uint64(vramSize), nil
}