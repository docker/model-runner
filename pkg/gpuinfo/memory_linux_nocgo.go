//go:build linux && !cgo

package gpuinfo

import "errors"

// getVRAMSize returns total system GPU memory in bytes
func getVRAMSize(_ string) (uint64, error) {
	return 0, errors.New("unimplemented without cgo")
}
