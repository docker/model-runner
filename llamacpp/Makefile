ifeq ($(OS),Windows_NT)
    DETECTED_OS := Windows
else
    UNAME_S := $(shell uname -s)
    ifeq ($(UNAME_S),Linux)
        DETECTED_OS := Linux
    endif
    ifeq ($(UNAME_S),Darwin)
        DETECTED_OS := macOS
    endif
endif

BUILD_DIR := build
INSTALL_DIR := install
NATIVE_DIR := native

.PHONY: build clean install-deps build-dir install-dir

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
	@echo "Windows build not implemented yet"
	@exit 1
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
	@echo "Windows dependency installation not implemented yet"
	@exit 1
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
	@echo "  build      	- Build llama.cpp (macOS only for now)"
	@echo "  install-deps	- Install build dependencies"
	@echo "  build-dir		- Print build directory path"
	@echo "  install-dir	- Print install directory path"
	@echo "  clean       	- Clean build artifacts"
	@echo "  help        	- Show this help"
