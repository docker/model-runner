#include <stdio.h>
#include "nvapi.h"

#pragma comment(lib, "nvapi64.lib")
int main() {
    NvAPI_Status status = NVAPI_OK;
    NvAPI_ShortString error_str = { 0 };

    status = NvAPI_Initialize();
    if (status != NVAPI_OK) {
        NvAPI_GetErrorMessage(status, error_str);
        printf("Failed to initialise NVAPI: %s\n", error_str);
        return -1;
    }

    NvU32 driver_version;
    NvAPI_ShortString build_branch;

    status = NvAPI_SYS_GetDriverAndBranchVersion(&driver_version, build_branch);
    if (status != NVAPI_OK) {
        NvAPI_GetErrorMessage(status, error_str);
        printf("Failed to retrieve driver info: %s\n", error_str);
        return -1;
    }

    printf("driver version: %u\n", driver_version);
    printf("build branch string: %s\n", build_branch);

    NV_PHYSICAL_GPUS_V1 nvPhysicalGPUs = { 0 };
    nvPhysicalGPUs.version = NV_PHYSICAL_GPUS_VER1;

    status = NvAPI_SYS_GetPhysicalGPUs(&nvPhysicalGPUs);
    if (status != NVAPI_OK) {
        NvAPI_GetErrorMessage(status, error_str);
        printf("Failed to retrieve physical GPU descriptors: %s\n", error_str);
        return -1;
    }

    for (NvU32 i = 0; i < nvPhysicalGPUs.gpuHandleCount; i++) {
        NvPhysicalGpuHandle gpu = nvPhysicalGPUs.gpuHandleData[i].hPhysicalGpu;

        NvAPI_ShortString gpu_name = { 0 };
        status = NvAPI_GPU_GetFullName(gpu, gpu_name);
        if (status == NVAPI_OK) {
            printf("GPU[%d]: full name: %s\n", i, gpu_name);
        } else {
            printf("GPU[%d]: full name: error\n", i);
        }

        NvU32 devid;
        NvU32 subsysid;
        NvU32 revid;
        NvU32 extid;
        status = NvAPI_GPU_GetPCIIdentifiers(gpu, &devid, &subsysid, &revid, &extid);
        if (status == NVAPI_OK) {
            printf("GPU[%d]: pci ids: device_id: 0x%04x; subsystem_id: 0x%04x; revision_id: 0x%04x; ext_device_id: 0x%04x\n",
                i, devid, subsysid, revid, extid);
        } else {
            printf("GPU[%d]: pci ids: error\n", i);
        }

        NV_GPU_MEMORY_INFO_EX_V1 nvMemoryInfo = { 0 };
        nvMemoryInfo.version = NV_GPU_MEMORY_INFO_EX_VER_1;

        status = NvAPI_GPU_GetMemoryInfoEx(gpu, &nvMemoryInfo);
        if (status == NVAPI_OK) {
            printf("GPU[%d]: dedicated memory: %lld\n",
                i, nvMemoryInfo.dedicatedVideoMemory);
        } else {
            printf("GPU[%d]: dedicated memory: error\n", i);
        }
    }

    return 0;
}
