# Native llama-server

## Building

    cmake -B build
    cmake --build build --parallel 8 --config Release

## Running

    ./build/bin/com.docker.llama-server --model <path to model>

## Bumping llama.cpp version

### Overview

This project uses llama.cpp as a git submodule located at `vendor/llama.cpp`, which points to the official llama.cpp repository at https://github.com/ggml-org/llama.cpp.git.

The project applies custom patches to llama.cpp's server implementation (`server.cpp` and `utils.hpp`) to integrate with the Docker model-runner architecture. These patches are maintained in `src/server/server.patch`.

### Prerequisites

Before bumping the version, ensure the submodule is initialized. **Run this command from the project root directory** (e.g., `/path/to/model-runner`):

```bash
# From the project root directory
git submodule update --init --recursive
```

If the submodule is already initialized, this command is safe to run and will ensure it's up to date.

**Note:** This command must be executed from the repository root because it needs to access the `.gitmodules` file and the submodule paths are relative to the root directory.

### Step-by-Step Process

1. **Find the desired llama.cpp version:**

   Visit https://github.com/ggml-org/llama.cpp/releases to find the latest stable release or a specific version you want to use. We typically bump to the latest tagged commit (e.g., `b1234`, `b2345`, etc.).

2. **Update the submodule to the desired version:**

   ```bash
   pushd vendor/llama.cpp
   git fetch origin
   git checkout <desired llama.cpp sha> # usually we bump to the latest tagged commit
   popd
   ```

3. **Apply the custom llama-server patch:**

   ```bash
   make -C src/server clean
   make -C src/server
   ```

   This will:
   - Clean the previous patched files
   - Copy the new `server.cpp` and `utils.hpp` from the updated llama.cpp
   - Apply our custom patches from `src/server/server.patch`

4. **Build and test:**

   ```bash
   # Build from the native directory   
    cmake -B build
    cmake --build build --parallel 8 --config Release
   
   # Test the build
   ./build/bin/com.docker.llama-server --model <path to model>
   ```

   Make sure everything builds cleanly without errors.

5. **Commit the submodule update:**

   ```bash
   git add vendor/llama.cpp
   git commit -m "Bump llama.cpp to <version>"
   ```
