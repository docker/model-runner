// Package runtime provides container runtime functionality for dmrlet.
package runtime

import (
	"os"
	"path/filepath"
	"strings"
)

// GPUInfo contains information about detected GPUs.
type GPUInfo struct {
	Type    string   // "nvidia", "amd", or "none"
	Devices []string // Device paths (e.g., /dev/nvidia0, /dev/dri/renderD128)
}

// DetectGPU detects available GPUs on the system.
func DetectGPU() *GPUInfo {
	// Try NVIDIA first
	if info := detectNVIDIA(); info != nil {
		return info
	}

	// Try AMD
	if info := detectAMD(); info != nil {
		return info
	}

	return &GPUInfo{Type: "none"}
}

// detectNVIDIA checks for NVIDIA GPUs.
func detectNVIDIA() *GPUInfo {
	// Check for nvidia-smi or NVIDIA devices
	devices := findDevices("/dev", "nvidia")
	if len(devices) == 0 {
		// Check for nvidia-smi in PATH
		if _, err := os.Stat("/usr/bin/nvidia-smi"); os.IsNotExist(err) {
			return nil
		}
		// nvidia-smi exists but no devices found yet - use all
		devices = []string{"/dev/nvidia0", "/dev/nvidiactl", "/dev/nvidia-uvm", "/dev/nvidia-uvm-tools"}
	}

	// Filter to only existing devices
	var existingDevices []string
	for _, d := range devices {
		if _, err := os.Stat(d); err == nil {
			existingDevices = append(existingDevices, d)
		}
	}

	if len(existingDevices) == 0 {
		return nil
	}

	return &GPUInfo{
		Type:    "nvidia",
		Devices: existingDevices,
	}
}

// detectAMD checks for AMD GPUs (ROCm).
func detectAMD() *GPUInfo {
	// Check for /dev/kfd (ROCm kernel driver)
	if _, err := os.Stat("/dev/kfd"); os.IsNotExist(err) {
		return nil
	}

	devices := []string{"/dev/kfd"}

	// Find render nodes
	renderDevices := findDevices("/dev/dri", "renderD")
	devices = append(devices, renderDevices...)

	// Also add /dev/dri/card* devices
	cardDevices := findDevices("/dev/dri", "card")
	devices = append(devices, cardDevices...)

	if len(renderDevices) == 0 {
		return nil
	}

	return &GPUInfo{
		Type:    "amd",
		Devices: devices,
	}
}

// findDevices finds device files matching a prefix in a directory.
func findDevices(dir, prefix string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var devices []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			devices = append(devices, filepath.Join(dir, entry.Name()))
		}
	}
	return devices
}

// GPUEnvVars returns environment variables needed for GPU support.
func GPUEnvVars(gpu *GPUInfo) []string {
	switch gpu.Type {
	case "nvidia":
		return []string{
			"NVIDIA_VISIBLE_DEVICES=all",
			"NVIDIA_DRIVER_CAPABILITIES=compute,utility",
		}
	case "amd":
		return []string{
			"HSA_OVERRIDE_GFX_VERSION=10.3.0",
			"ROCM_PATH=/opt/rocm",
		}
	default:
		return nil
	}
}
