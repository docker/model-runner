# Native llama-server

## Building

    cmake -B build
    cmake --build build --parallel 8 --config Release

## Running

    ./build/bin/com.docker.llama-server --model <path to model>

## Bumping llama.cpp version

1. Pull and checkout the desired llama.cpp version:

```
pushd vendor/llama.cpp
git fetch origin
git checkout <desired llama.cpp sha> # usually we bump to the latest tagged commit
popd
```

2. Apply our llama-server patch:

```
make -C src/server clean
make -C src/server
```

3. Make sure everyting builds cleanly following the update.
