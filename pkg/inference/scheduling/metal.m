// +build darwin

#include <Metal/Metal.h>

#include "metal.h"

size_t getVRAMSize() {
    id<MTLDevice> device = MTLCreateSystemDefaultDevice();
    if (device) {
        return [device recommendedMaxWorkingSetSize];
    }
    return 0;
}