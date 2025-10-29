//go:build linux

package llamacpp

import (
	"context"
	"fmt"
	"net/http"

	"github.com/docker/model-runner/pkg/gpuinfo"
	"github.com/docker/model-runner/pkg/logging"
)

func init() {
	// Enable GPU variant detection by default on Linux
	ShouldUseGPUVariantLock.Lock()
	defer ShouldUseGPUVariantLock.Unlock()
	ShouldUseGPUVariant = true
}

func (l *llamaCpp) ensureLatestLlamaCpp(ctx context.Context, log logging.Logger, httpClient *http.Client,
	llamaCppPath, vendoredServerStoragePath string,
) error {
	var hasAMD bool
	var hasMTHREADS bool
	var err error

	ShouldUseGPUVariantLock.Lock()
	defer ShouldUseGPUVariantLock.Unlock()
	if ShouldUseGPUVariant {
		// Create GPU info to check for supported AMD GPUs
		gpuInfo := gpuinfo.New(vendoredServerStoragePath)
		hasAMD, err = gpuInfo.HasSupportedAMDGPU()
		if err != nil {
			log.Debugf("AMD GPU detection failed: %v", err)
		}

		hasMTHREADS, err = gpuInfo.HasSupportedMTHREADSGPU()
		if err != nil {
			log.Debugf("MTHREADS GPU detection failed: %v", err)
		}
	}

	desiredVersion := GetDesiredServerVersion()
	desiredVariant := "cpu"

	// Use ROCm if supported AMD GPU is detected
	if hasAMD {
		log.Info("Supported AMD GPU detected, using ROCm variant")
		desiredVariant = "rocm"
	}

	// USE MUSA if supported MTHREADS GPU is detected
	if hasMTHREADS {
		log.Info("Supported MTHREADS GPU detected, using MUSA variant")
		desiredVariant = "musa"
	}

	l.status = fmt.Sprintf("looking for updates for %s variant", desiredVariant)
	return l.downloadLatestLlamaCpp(ctx, log, httpClient, llamaCppPath, vendoredServerStoragePath, desiredVersion,
		desiredVariant)
}
