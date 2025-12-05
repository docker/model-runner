include ../Makefile.common

BUILD_DIR := build
INSTALL_DIR := install
NATIVE_DIR := native

# Vulkan SDK version for Windows
VULKAN_VERSION := 1.4.313.2

# Architecture detection for Windows
ifeq ($(DETECTED_OS),Windows)
    UNAME_M := $(shell uname -m)
    ifeq ($(UNAME_M),x86_64)
        DETECTED_ARCH := amd64
    else ifeq ($(UNAME_M),aarch64)
        DETECTED_ARCH := arm64
    else ifeq ($(UNAME_M),arm64)
        DETECTED_ARCH := arm64
    else
        DETECTED_ARCH := amd64
    endif
endif

.PHONY: build clean install-deps install-deps-cuda install-deps-opencl build-dir install-dir

build: install-deps
ifeq ($(DETECTED_OS),macOS)
	@echo "Building for macOS..."
	@echo "Configuring CMake..."
	cmake -B $(BUILD_DIR) \
		-DCMAKE_CXX_COMPILER=clang++ \
		-DCMAKE_C_COMPILER=clang \
		-DCMAKE_BUILD_TYPE=Release \
		-DCMAKE_OSX_DEPLOYMENT_TARGET=13.3 \
		-DCMAKE_MACOSX_RPATH=ON \
		-DCMAKE_INSTALL_RPATH='@executable_path/../lib' \
		-DGGML_NATIVE=OFF \
		-DGGML_OPENMP=OFF \
		-DLLAMA_CURL=OFF \
		-GNinja \
		-S $(NATIVE_DIR)
	@echo "Building..."
	cmake --build $(BUILD_DIR) --config Release
	@echo "Installing..."
	cmake --install $(BUILD_DIR) \
		--config Release \
		--prefix $(INSTALL_DIR)
	@echo "Cleaning install directory..."
	rm $(INSTALL_DIR)/bin/*.py
	rm -rf $(INSTALL_DIR)/lib/cmake
	rm -rf $(INSTALL_DIR)/lib/pkgconfig
	rm -rf $(INSTALL_DIR)/include
	@echo "Build complete! Binaries are in $(INSTALL_DIR)"
else ifeq ($(DETECTED_OS),Linux)
	@echo "Linux build not implemented yet"
	@exit 1
else ifeq ($(DETECTED_OS),Windows)
ifeq ($(DETECTED_ARCH),amd64)
	@echo "Building for Windows AMD64 (CPU with Vulkan)..."
	@echo "Configuring CMake..."
	PATH="C:/Program Files/LLVM/bin;$$PATH" cmake -B $(BUILD_DIR) \
		-DCMAKE_BUILD_TYPE=Release \
		-DGGML_NATIVE=OFF \
		-DGGML_OPENMP=OFF \
		-DDDLLAMA_BUILD_UTILS=ON \
		-GNinja \
		-DLLAMA_CURL=OFF \
		-DBUILD_SHARED_LIBS=ON \
		-DGGML_BACKEND_DL=ON \
		-DGGML_CPU_ALL_VARIANTS=ON \
		-DGGML_VULKAN=ON \
		"-DCMAKE_C_COMPILER=C:/Program Files/LLVM/bin/clang.exe" \
		"-DCMAKE_CXX_COMPILER=C:/Program Files/LLVM/bin/clang++.exe" \
		"-DCMAKE_RC_COMPILER=C:/Program Files/LLVM/bin/llvm-rc.exe" \
		-S $(NATIVE_DIR)
	@echo "Building..."
	PATH="C:/Program Files/LLVM/bin;$$PATH" cmake --build $(BUILD_DIR) --config Release
	@echo "Installing..."
	cmake --install $(BUILD_DIR) \
		--config Release \
		--prefix $(INSTALL_DIR)
	@echo "Cleaning install directory..."
	rm -f $(INSTALL_DIR)/bin/*.py
	rm -rf $(INSTALL_DIR)/lib/cmake
	rm -rf $(INSTALL_DIR)/lib/pkgconfig
	rm -rf $(INSTALL_DIR)/include
	@echo "Build complete! Binaries are in $(INSTALL_DIR)"
else
	@echo "Building for Windows ARM64 (CPU)..."
	@echo "Configuring CMake..."
	cmake -B $(BUILD_DIR) \
		-G Ninja \
		-DCMAKE_C_COMPILER="C:/Program Files/LLVM/bin/clang.exe" \
		-DCMAKE_CXX_COMPILER="C:/Program Files/LLVM/bin/clang++.exe" \
		-DCMAKE_BUILD_TYPE=Release \
		-DGGML_NATIVE=OFF \
		-DLLAMA_CURL=OFF \
		-S $(NATIVE_DIR)
	@echo "Building..."
	cmake --build $(BUILD_DIR) --config Release
	@echo "Installing..."
	cmake --install $(BUILD_DIR) \
		--config Release \
		--prefix $(INSTALL_DIR)
	@echo "Cleaning install directory..."
	rm -f $(INSTALL_DIR)/bin/*.py
	rm -rf $(INSTALL_DIR)/lib/cmake
	rm -rf $(INSTALL_DIR)/lib/pkgconfig
	rm -rf $(INSTALL_DIR)/include
	@echo "Build complete! Binaries are in $(INSTALL_DIR)"
endif
else
	@echo "Unsupported OS: $(DETECTED_OS)"
	@exit 1
endif

install-deps:
ifeq ($(DETECTED_OS),macOS)
	@echo "Installing build dependencies for macOS..."
	@if ! command -v ninja >/dev/null 2>&1; then \
		echo "Installing Ninja..."; \
		brew install ninja; \
	else \
		echo "Ninja already installed"; \
	fi
else ifeq ($(DETECTED_OS),Linux)
	@echo "Linux dependency installation not implemented yet"
	@exit 1
else ifeq ($(DETECTED_OS),Windows)
ifeq ($(DETECTED_ARCH),amd64)
	@echo "Installing build dependencies for Windows AMD64 (CPU with Vulkan)..."
	@echo "Installing Ninja..."
	choco install ninja -y
	@echo "Installing CMake..."
	choco install cmake -y --installargs 'ADD_CMAKE_TO_PATH=System'
	@echo "Installing LLVM (clang)..."
	choco install llvm -y
	@echo "Adding LLVM to system PATH..."
	@powershell -Command "[System.Environment]::SetEnvironmentVariable('PATH', 'C:\\Program Files\\LLVM\\bin;' + [System.Environment]::GetEnvironmentVariable('PATH', 'Machine'), 'Machine')" || echo "LLVM PATH update attempted"
	@if [ -d "/c/VulkanSDK/$(VULKAN_VERSION)" ]; then \
		echo "Vulkan SDK $(VULKAN_VERSION) already installed"; \
	else \
		echo "Installing Vulkan SDK $(VULKAN_VERSION)..."; \
		curl.exe -o "$$TEMP/VulkanSDK-Installer.exe" -L "https://sdk.lunarg.com/sdk/download/$(VULKAN_VERSION)/windows/vulkansdk-windows-X64-$(VULKAN_VERSION).exe"; \
		"$$TEMP/VulkanSDK-Installer.exe" --accept-licenses --default-answer --confirm-command install; \
		powershell -Command "[System.Environment]::SetEnvironmentVariable('VULKAN_SDK', 'C:\\VulkanSDK\\$(VULKAN_VERSION)', 'User')"; \
		powershell -Command "[System.Environment]::SetEnvironmentVariable('PATH', [System.Environment]::GetEnvironmentVariable('PATH', 'User') + ';C:\\VulkanSDK\\$(VULKAN_VERSION)\\bin', 'User')"; \
		echo "Vulkan SDK $(VULKAN_VERSION) installed. Please restart your shell for changes to take effect."; \
	fi
else
	@echo "Installing build dependencies for Windows ARM64 (CPU)..."
	@echo "Ninja and LLVM should already be installed on ARM64 runners"
endif
else
	@echo "Unsupported OS: $(DETECTED_OS)"
	@exit 1
endif

clean:
	rm -rf $(BUILD_DIR)
	rm -rf $(INSTALL_DIR)

build-dir:
	@echo "$(shell pwd)/$(BUILD_DIR)"

install-dir:
	@echo "$(shell pwd)/$(INSTALL_DIR)"

help:
	@echo "Available targets:"
	@echo "  build           - Build llama.cpp (macOS and Windows)"
	@echo "  install-deps    - Install build dependencies (CPU with Vulkan for Windows AMD64)"
	@echo "  build-dir       - Print build directory path"
	@echo "  install-dir     - Print install directory path"
	@echo "  clean           - Clean build artifacts"
	@echo "  help            - Show this help"
