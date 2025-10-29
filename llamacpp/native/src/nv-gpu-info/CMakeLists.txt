set(TARGET com.docker.nv-gpu-info)

add_library(nvapi STATIC IMPORTED)
set_target_properties(nvapi PROPERTIES
  IMPORTED_LOCATION "${CMAKE_SOURCE_DIR}/vendor/nvapi/amd64/nvapi64.lib"
  INTERFACE_INCLUDE_DIRECTORIES "${CMAKE_SOURCE_DIR}/vendor/nvapi"
)

add_executable(${TARGET} nv-gpu-info.c)
install(TARGETS ${TARGET} RUNTIME)

target_link_libraries(${TARGET} nvapi)