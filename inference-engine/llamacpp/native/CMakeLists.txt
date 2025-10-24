cmake_minimum_required(VERSION 3.13)

project(
    com.docker.llama-server.native
    DESCRIPTION "DD inference server, based on llama.cpp native server"
    LANGUAGES C CXX
)

option(DDLLAMA_BUILD_SERVER "Build the DD llama.cpp server executable" ON)
option(DDLLAMA_BUILD_UTILS "Build utilities, e.g. nv-gpu-info" OFF)
set(DDLLAMA_PATCH_COMMAND "patch" CACHE STRING "patch command")

set(CMAKE_RUNTIME_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}/bin)
set(CMAKE_LIBRARY_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}/bin)

if (DDLLAMA_BUILD_SERVER)
    set(LLAMA_BUILD_COMMON ON)
    add_subdirectory(vendor/llama.cpp)
    add_subdirectory(vendor/llama.cpp/tools/mtmd)
    add_subdirectory(src/server)
endif()

if (WIN32 AND DDLLAMA_BUILD_UTILS)
    add_subdirectory(src/nv-gpu-info)
endif()
