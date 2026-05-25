cmake_minimum_required(VERSION 3.13)

project(
    com.docker.llama-server.native
    DESCRIPTION "DD inference server, based on llama.cpp native server"
    LANGUAGES C CXX
)

option(DDLLAMA_BUILD_SERVER "Build the DD llama.cpp server executable" ON)
option(DDLLAMA_BUILD_UTILS "Build utilities, e.g. nv-gpu-info" OFF)

set(CMAKE_RUNTIME_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}/bin)
set(CMAKE_LIBRARY_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}/bin)

if (DDLLAMA_BUILD_SERVER)
    # Build upstream llama.cpp with server enabled
    # Only set these options if they're not already defined to allow consumers to override
    if(NOT DEFINED LLAMA_BUILD_COMMON)
        set(LLAMA_BUILD_COMMON ON CACHE BOOL "Build common utils library")
    endif()
    if(NOT DEFINED LLAMA_BUILD_TOOLS)
        set(LLAMA_BUILD_TOOLS ON CACHE BOOL "Build tools")
    endif()
    if(NOT DEFINED LLAMA_BUILD_SERVER)
        set(LLAMA_BUILD_SERVER ON CACHE BOOL "Build server")
    endif()
    add_subdirectory(vendor/llama.cpp)

    # Ensure shared library DLLs are installed on Windows.
    # Upstream install rules use LIBRARY which doesn't install RUNTIME (DLL) artifacts on DLL platforms.
    if (BUILD_SHARED_LIBS)
        install(TARGETS llama RUNTIME DESTINATION bin)
        if (TARGET llama-common)
            install(TARGETS llama-common RUNTIME DESTINATION bin)
        endif()
    endif()

    # Create custom target to copy llama-server to com.docker.llama-server
    if (WIN32)
        set(LLAMA_SERVER_DST "${CMAKE_RUNTIME_OUTPUT_DIRECTORY}/com.docker.llama-server.exe")
    else()
        set(LLAMA_SERVER_DST "${CMAKE_RUNTIME_OUTPUT_DIRECTORY}/com.docker.llama-server")
    endif()

    add_custom_command(OUTPUT "${LLAMA_SERVER_DST}"
        COMMAND ${CMAKE_COMMAND} -E copy "$<TARGET_FILE:llama-server>" "${LLAMA_SERVER_DST}"
        DEPENDS llama-server
        COMMENT "Creating com.docker.llama-server from llama-server"
    )

    add_custom_target(com.docker.llama-server ALL DEPENDS "${LLAMA_SERVER_DST}")

    # Install the renamed binary
    install(PROGRAMS "${LLAMA_SERVER_DST}" DESTINATION bin)
endif()

if (WIN32 AND DDLLAMA_BUILD_UTILS)
    add_subdirectory(src/nv-gpu-info)
endif()
