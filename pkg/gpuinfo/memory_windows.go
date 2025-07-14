package gpuinfo

import (
	"bufio"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// getVRAMSize returns total system GPU memory in bytes
func getVRAMSize(ctx context.Context, modelRuntimeInstallPath string) (uint64, error) {
	nvGPUInfoBin := filepath.Join(modelRuntimeInstallPath, "com.docker.nv-gpu-info.exe")

	cmd := exec.CommandContext(ctx, nvGPUInfoBin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, err
	}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		vram, found := strings.CutPrefix(sc.Text(), "GPU[0]: dedicated memory:")
		if found {
			vram = strings.TrimSpace(vram)
			return strconv.ParseUint(vram, 10, 64)
		}
	}
	return 0, errors.New("unexpected nv-gpu-info output format")
}
