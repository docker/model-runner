package gpuinfo

type GPUInfo struct{}

func New() *GPUInfo {
	return &GPUInfo{}
}

func (g *GPUInfo) GetVRAMSize() (uint64, error) {
	return getVRAMSize()
}
