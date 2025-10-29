package gpuinfo

import (
	"bufio"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// getVRAMSize returns total system GPU memory in bytes
func getVRAMSize(modelRuntimeInstallPath string) (uint64, error) {
	if runtime.GOARCH == "arm64" {
		// TODO(p1-0tr): For now, on windows/arm64, stick to the old behaviour. This will
		// require backend.GetRequiredMemoryForModel to return 1 as well.
		return 1, nil
	}

	nvGPUInfoBin := filepath.Join(modelRuntimeInstallPath, "bin", "com.docker.nv-gpu-info.exe")

	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
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
