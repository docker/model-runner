package memory

import (
	"errors"

	"github.com/docker/model-runner/pkg/gpuinfo"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/logging"
	"github.com/elastic/go-sysinfo"
)

type SystemMemoryInfo interface {
	HaveSufficientMemory(inference.RequiredMemory) (bool, error)
	GetTotalMemory() inference.RequiredMemory
}

type systemMemoryInfo struct {
	log         logging.Logger
	totalMemory inference.RequiredMemory
}

func NewSystemMemoryInfo(log logging.Logger, gpuInfo *gpuinfo.GPUInfo) (SystemMemoryInfo, error) {
	// Compute the amount of available memory.
	// TODO(p1-0tr): improve error handling
	vramSize, err := gpuInfo.GetVRAMSize()
	if err != nil {
		vramSize = 1
		log.Warnf("Could not read VRAM size: %s", err)
	} else {
		log.Infof("Running on system with %d MB VRAM", vramSize/1024/1024)
	}
	ramSize := uint64(1)
	hostInfo, err := sysinfo.Host()
	if err != nil {
		log.Warnf("Could not read host info: %s", err)
	} else {
		ram, err := hostInfo.Memory()
		if err != nil {
			log.Warnf("Could not read host RAM size: %s", err)
		} else {
			ramSize = ram.Total
			log.Infof("Running on system with %d MB RAM", ramSize/1024/1024)
		}
	}
	return &systemMemoryInfo{
		log:         log,
		totalMemory: inference.RequiredMemory{RAM: ramSize, VRAM: vramSize},
	}, nil
}

func (s *systemMemoryInfo) HaveSufficientMemory(req inference.RequiredMemory) (bool, error) {
	// Sentinel value of 1 indicates unknown RAM/VRAM
	if req.RAM > 1 && s.totalMemory.RAM == 1 {
		return false, errors.New("system RAM unknown")
	}
	if req.VRAM > 1 && s.totalMemory.VRAM == 1 {
		return false, errors.New("system VRAM unknown")
	}
	return req.RAM <= s.totalMemory.RAM && req.VRAM <= s.totalMemory.VRAM, nil
}

func (s *systemMemoryInfo) GetTotalMemory() inference.RequiredMemory {
	return s.totalMemory
}
