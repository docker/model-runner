package gpuinfo

type GPUInfo struct {
	// modelRuntimeInstallPath is the location where DMR installed it's llama-server
	// and accompanying tools
	modelRuntimeInstallPath string
}

func New(modelRuntimeInstallPath string) *GPUInfo {
	return &GPUInfo{
		modelRuntimeInstallPath: modelRuntimeInstallPath,
	}
}

func (g *GPUInfo) GetVRAMSize() (uint64, error) {
	return getVRAMSize(g.modelRuntimeInstallPath)
}
