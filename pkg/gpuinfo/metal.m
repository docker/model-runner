//go:build darwin

#include <Metal/Metal.h>

#include "metal.h"

size_t getVRAMSize() {
    id<MTLDevice> device = MTLCreateSystemDefaultDevice();
    if (device) {
        size_t vramsz = [device recommendedMaxWorkingSetSize];
        [device release];
        return vramsz;
    }
    return 0;
}