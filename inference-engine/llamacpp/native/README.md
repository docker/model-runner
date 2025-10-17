# Native llama-server for DD

## Building

    cmake -B build
    cmake --build build --parallel 8 --config Release

## Running

    DD_INF_UDS=<socket path> ./build/bin/com.docker.llama-server --model <path to model>
