package scheduling

/*
#include "nvapi.h"
*/
import "C"
import "errors"

// getVRAMSize returns total system GPU memory in bytes
func getVRAMSize() (uint64, error) {
	vramSize := C.getVRAMSize()
	if vramSize == 0 {
		return 0, errors.New("could not get nvapi VRAM size")
	}
	return uint64(vramSize), nil
}