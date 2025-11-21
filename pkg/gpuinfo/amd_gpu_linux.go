//go:build linux

package gpuinfo

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// supportedAMDGPUs are the AMD GPU targets that should use ROCm
var supportedAMDGPUs = map[string]bool{
	"gfx908":  true,
	"gfx90a":  true,
	"gfx942":  true,
	"gfx1010": true,
	"gfx1030": true,
	"gfx1100": true,
	"gfx1200": true,
	"gfx1201": true,
	"gfx1151": true,
}

func hasSupportedAMDGPU() (bool, error) {
	// Check if KFD topology directory exists
	topologyDir := "/sys/class/kfd/kfd/topology/nodes/"
	info, err := os.Stat(topologyDir)
	if err != nil || !info.IsDir() {
		return false, nil // KFD not available
	}

	entries, err := os.ReadDir(topologyDir)
	if err != nil {
		return false, err
	}

	// Sort entries by name to maintain consistent order
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	// Compile regex to match gfx_target_version lines
	reTarget := regexp.MustCompile(`gfx_target_version[ \t]+([0-9]+)`)

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		nodePath := filepath.Join(topologyDir, e.Name())
		propPath := filepath.Join(nodePath, "properties")

		// Attempt to open the properties file directly; skip on error (e.g., permissions)
		f, err := os.Open(propPath)
		if err != nil {
			// Could be permission denied or file doesn't exist; just skip like the Python code
			continue
		}

		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := sc.Text()
			matches := reTarget.FindStringSubmatch(line)
			if len(matches) < 2 {
				continue
			}

			deviceIDStr := matches[1]
			deviceID, err := strconv.Atoi(deviceIDStr)
			if err != nil || deviceID == 0 {
				continue
			}

			var majorVer, minorVer, steppingVer int
			if gfxOverride := os.Getenv("HSA_OVERRIDE_GFX_VERSION"); gfxOverride != "" {
				parts := strings.Split(strings.TrimSpace(gfxOverride), ".")
				if len(parts) != 3 {
					// Invalid format, skip
					continue
				}
				mv, err1 := strconv.Atoi(parts[0])
				nv, err2 := strconv.Atoi(parts[1])
				sv, err3 := strconv.Atoi(parts[2])
				if err1 != nil || err2 != nil || err3 != nil {
					// Invalid format, skip
					continue
				}
				if mv > 63 || nv > 255 || sv > 255 {
					// Invalid values, skip
					continue
				}
				majorVer, minorVer, steppingVer = mv, nv, sv
			} else {
				majorVer = (deviceID / 10000) % 100
				minorVer = (deviceID / 100) % 100
				steppingVer = deviceID % 100
			}

			gfx := "gfx" +
				strconv.FormatInt(int64(majorVer), 10) +
				strconv.FormatInt(int64(minorVer), 16) +
				strconv.FormatInt(int64(steppingVer), 16)

			if supportedAMDGPUs[gfx] {
				f.Close()
				return true, nil // Found a supported AMD GPU
			}
		}
		f.Close()
	}

	return false, nil // No supported AMD GPU found
}