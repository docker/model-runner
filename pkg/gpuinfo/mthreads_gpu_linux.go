//go:build linux

package gpuinfo

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

func hasSupportedMTHREADSGPU() (bool, error) {
	// Check if /dev contains mtgpu.* devices
	devDir := "/dev"
	devEntries, err := os.ReadDir(devDir)
	if err != nil {
		return false, err
	}

	foundMTGPU := false
	for _, entry := range devEntries {
		if strings.HasPrefix(entry.Name(), "mtgpu") {
			// MTGPU driver should be properly installed and loaded
			foundMTGPU = true
			break
		}
	}
	if !foundMTGPU {
		return false, nil // no MTHREADS GPU device found
	}

	// Run muInfo to collect GPU information
	cmd := exec.Command("muInfo")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return false, errors.New("failed to execute muInfo: " + err.Error() + "\n" + out.String())
	}

	// Parse the output
	scanner := bufio.NewScanner(&out)
	reDriver := regexp.MustCompile(`Driver Version:[ \t]+([0-9.]+)`)
	reCompute := regexp.MustCompile(`compute capability:[ \t]+([0-9.]+)`)

	var driverVerStr, computeCapStr string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if m := reDriver.FindStringSubmatch(line); len(m) == 2 {
			driverVerStr = m[1]
		}
		if m := reCompute.FindStringSubmatch(line); len(m) == 2 {
			computeCapStr = m[1]
		}
	}

	if driverVerStr == "" || computeCapStr == "" {
		return false, errors.New("failed to parse muInfo output for driver version or compute capability")
	}

	// Helper to parse float version (e.g., 4.3, 2.1)
	parseVersion := func(s string) float64 {
		v, _ := strconv.ParseFloat(s, 64)
		return v
	}

	driverVer := parseVersion(driverVerStr)
	computeCap := parseVersion(computeCapStr)

	// Check minimum supported versions
	if driverVer >= 4.3 && computeCap >= 2.1 {
		return true, nil
	}

	return false, nil // No supported MTHREADS GPU found
}
