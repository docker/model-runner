// +build windows

#include "nvapi.h"

typedef enum {
    NVAPI_OK = 0
} NvAPI_Status;

typedef unsigned int NvU32;
typedef struct {
    NvU32 version;
    NvU32 dedicatedVideoMemory;
    NvU32 availableDedicatedVideoMemory;
    NvU32 systemVideoMemory;
    NvU32 sharedSystemMemory;
} NV_DISPLAY_DRIVER_MEMORY_INFO;

typedef void* NvPhysicalGpuHandle;

#define NV_DISPLAY_DRIVER_MEMORY_INFO_VER 0x10028

size_t getVRAMSize() {
    HMODULE handle;
    NvAPI_Status (*NvAPI_Initialize)(void);
    NvAPI_Status (*NvAPI_EnumPhysicalGPUs)(NvPhysicalGpuHandle* handles, NvU32* count);
    NvAPI_Status (*NvAPI_GPU_GetMemoryInfo)(NvPhysicalGpuHandle handle, NV_DISPLAY_DRIVER_MEMORY_INFO* memInfo);
    NvAPI_Status (*NvAPI_Unload)(void);
    
    NvAPI_Status status;
    NvPhysicalGpuHandle handles[64];
    NvU32 count = 0;
    NV_DISPLAY_DRIVER_MEMORY_INFO memInfo;
    
    // Try to load nvapi64.dll first, then fallback to nvapi.dll
    handle = LoadLibraryA("nvapi64.dll");
    if (!handle) {
        handle = LoadLibraryA("nvapi.dll");
        if (!handle) {
            return 0;
        }
    }
    
    // Load required functions
    NvAPI_Initialize = (NvAPI_Status(*)(void))GetProcAddress(handle, "NvAPI_Initialize");
    NvAPI_EnumPhysicalGPUs = (NvAPI_Status(*)(NvPhysicalGpuHandle*, NvU32*))GetProcAddress(handle, "NvAPI_EnumPhysicalGPUs");
    NvAPI_GPU_GetMemoryInfo = (NvAPI_Status(*)(NvPhysicalGpuHandle, NV_DISPLAY_DRIVER_MEMORY_INFO*))GetProcAddress(handle, "NvAPI_GPU_GetMemoryInfo");
    NvAPI_Unload = (NvAPI_Status(*)(void))GetProcAddress(handle, "NvAPI_Unload");
    
    if (!NvAPI_Initialize || !NvAPI_EnumPhysicalGPUs || !NvAPI_GPU_GetMemoryInfo || !NvAPI_Unload) {
        FreeLibrary(handle);
        return 0;
    }
    
    status = NvAPI_Initialize();
    if (status != NVAPI_OK) {
        FreeLibrary(handle);
        return 0;
    }
    
    status = NvAPI_EnumPhysicalGPUs(handles, &count);
    if (status != NVAPI_OK || count == 0) {
        NvAPI_Unload();
        FreeLibrary(handle);
        return 0;
    }
    
    memInfo.version = NV_DISPLAY_DRIVER_MEMORY_INFO_VER;
    status = NvAPI_GPU_GetMemoryInfo(handles[0], &memInfo);
    if (status != NVAPI_OK) {
        NvAPI_Unload();
        FreeLibrary(handle);
        return 0;
    }
    
    NvAPI_Unload();
    FreeLibrary(handle);
    
    // Return dedicated video memory in bytes (convert from KB)
    return (size_t)memInfo.dedicatedVideoMemory * 1024;
}