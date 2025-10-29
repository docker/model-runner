//go:build linux

#include "nvidia.h"

typedef enum {
    NVML_SUCCESS = 0
} nvmlReturn_t;

typedef struct {
    unsigned long long total;
    unsigned long long free;
    unsigned long long used;
} nvmlMemory_t;

typedef void* nvmlDevice_t;

size_t getVRAMSize() {
    void* handle;
    nvmlReturn_t (*nvmlInit)(void);
    nvmlReturn_t (*nvmlShutdown)(void);
    nvmlReturn_t (*nvmlDeviceGetHandleByIndex)(unsigned int index, nvmlDevice_t* device);
    nvmlReturn_t (*nvmlDeviceGetMemoryInfo)(nvmlDevice_t device, nvmlMemory_t* memory);
    
    nvmlReturn_t result;
    nvmlDevice_t device;
    nvmlMemory_t memory;
    
    // Try to load libnvidia-ml.so.1 first, then fallback to libnvidia-ml.so
    handle = dlopen("libnvidia-ml.so.1", RTLD_LAZY);
    if (!handle) {
        handle = dlopen("libnvidia-ml.so", RTLD_LAZY);
        if (!handle) {
            return 0;
        }
    }
    
    // Load required functions
    nvmlInit = dlsym(handle, "nvmlInit");
    nvmlShutdown = dlsym(handle, "nvmlShutdown");
    nvmlDeviceGetHandleByIndex = dlsym(handle, "nvmlDeviceGetHandleByIndex");
    nvmlDeviceGetMemoryInfo = dlsym(handle, "nvmlDeviceGetMemoryInfo");
    
    if (!nvmlInit || !nvmlShutdown || !nvmlDeviceGetHandleByIndex || !nvmlDeviceGetMemoryInfo) {
        dlclose(handle);
        return 0;
    }
    
    result = nvmlInit();
    if (result != NVML_SUCCESS) {
        dlclose(handle);
        return 0;
    }
    
    result = nvmlDeviceGetHandleByIndex(0, &device);
    if (result != NVML_SUCCESS) {
        nvmlShutdown();
        dlclose(handle);
        return 0;
    }
    
    result = nvmlDeviceGetMemoryInfo(device, &memory);
    if (result != NVML_SUCCESS) {
        nvmlShutdown();
        dlclose(handle);
        return 0;
    }
    
    nvmlShutdown();
    dlclose(handle);
    return memory.total;
}