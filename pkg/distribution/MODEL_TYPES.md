# Model Types and Interfaces

This document explains the model types and interfaces in the distribution package.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         oci.Image                                │
│   (Low-level OCI artifact: Layers, Manifest, Digest, etc.)      │
└───────────────────────────────┬─────────────────────────────────┘
                                │ embeds
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                      types.ModelArtifact                        │
│         (Building & pushing: ID, Config, Descriptor)            │
└───────────────────────────────┬─────────────────────────────────┘
                                │ implemented by
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                      partial.BaseModel                          │
│        (Common implementation with LayerList, ConfigFile)       │
└─────────────────────────────────────────────────────────────────┘
```

## Core Interfaces

### `oci.Image` (pkg/distribution/oci/image.go)

Low-level OCI artifact operations for registry storage.

```go
type Image interface {
    Layers() ([]Layer, error)
    MediaType() (MediaType, error)
    Size() (int64, error)
    ConfigName() (Hash, error)
    ConfigFile() (*ConfigFile, error)
    RawConfigFile() ([]byte, error)
    Digest() (Hash, error)
    Manifest() (*Manifest, error)
    RawManifest() ([]byte, error)
    LayerByDigest(Hash) (Layer, error)
    LayerByDiffID(Hash) (Layer, error)
}
```

### `types.ModelArtifact` (pkg/distribution/types/model.go)

For building and distributing models. Extends `oci.Image`.

```go
type ModelArtifact interface {
    ID() (string, error)
    Config() (ModelConfig, error)
    Descriptor() (Descriptor, error)
    oci.Image
}
```

### `types.Model` (pkg/distribution/types/model.go)

Stored model with file path resolution for inference.

```go
type Model interface {
    ID() (string, error)
    GGUFPaths() ([]string, error)
    SafetensorsPaths() ([]string, error)
    DDUFPaths() ([]string, error)
    ConfigArchivePath() (string, error)
    MMPROJPath() (string, error)
    Config() (ModelConfig, error)
    Tags() []string
    Descriptor() (Descriptor, error)
    ChatTemplatePath() (string, error)
}
```

### `types.ModelBundle` (pkg/distribution/types/model.go)

Unpacked model ready for runtime execution.

```go
type ModelBundle interface {
    RootDir() string
    GGUFPath() string
    SafetensorsPath() string
    DDUFPath() string
    ChatTemplatePath() string
    MMPROJPath() string
    RuntimeConfig() ModelConfig
}
```

### `types.ModelConfig` (pkg/distribution/types/config.go)

Format-agnostic configuration access.

```go
type ModelConfig interface {
    GetFormat() Format
    GetContextSize() *int32
    GetSize() string
    GetArchitecture() string
    GetParameters() string
    GetQuantization() string
}
```

Implemented by:
- `*types.Config` (Docker format, snake_case JSON)
- `*modelpack.Model` (CNCF ModelPack format, camelCase JSON)

## Helper Interfaces (pkg/distribution/internal/partial/)

Compositional interfaces enabling code reuse:

```go
type WithRawConfigFile interface {
    RawConfigFile() ([]byte, error)
}

type WithRawManifest interface {
    RawManifest() ([]byte, error)
}

type WithLayers interface {
    WithRawConfigFile
    Layers() ([]oci.Layer, error)
}

type WithConfigMediaType interface {
    GetConfigMediaType() oci.MediaType
}
```

Helper functions work with any type satisfying these interfaces:
- `Config(WithRawConfigFile)` → `ModelConfig`
- `ID(WithRawManifest)` → `string`
- `GGUFPaths(WithLayers)` → `[]string`
- `SafetensorsPaths(WithLayers)` → `[]string`
- `ManifestForLayers(WithLayers)` → `*oci.Manifest`

## Concrete Types

### `partial.BaseModel`

Common implementation for model artifacts:

```go
type BaseModel struct {
    ModelConfigFile types.ConfigFile
    LayerList       []oci.Layer
    ConfigMediaType oci.MediaType
}
```

### `partial.Layer`

Local file layer implementing `oci.Layer`:

```go
type Layer struct {
    Path string
    oci.Descriptor
}
```

## Model Formats

| Format | Constant | Description |
|--------|----------|-------------|
| GGUF | `FormatGGUF` | llama.cpp quantized models |
| Safetensors | `FormatSafetensors` | Hugging Face weights |
| Diffusers | `FormatDiffusers` | Image generation models |
| Safetensors | `FormatSafetensors` | HuggingFace weights |
| Diffusers | `FormatDiffusers` | Image generation models |

## Media Types

### Docker Format
- `application/vnd.docker.ai.model.config.v0.1+json` - Legacy config
- `application/vnd.docker.ai.model.config.v0.2+json` - Layer-per-file config
- `application/vnd.docker.ai.gguf.v3` - GGUF weights
- `application/vnd.docker.ai.safetensors` - Safetensors weights

### CNCF ModelPack Format
- `application/vnd.cncf.model.config.v1+json` - ModelPack config
- `application/vnd.cncf.model.weight.v1.gguf` - GGUF weights
- `application/vnd.cncf.model.weight.v1.safetensors` - Safetensors weights

## Why Multiple Interfaces?

| Interface | Use Case |
|-----------|----------|
| `oci.Image` | Registry push/pull operations |
| `ModelArtifact` | Building and distributing models |
| `Model` | Stored model file path access |
| `ModelBundle` | Runtime execution |
| `ModelConfig` | Format-agnostic config access |

This separation enables:
- Different backends (llama.cpp, vLLM) consume the same `ModelBundle`
- Support for multiple formats (Docker, CNCF ModelPack) without conversion
- Clean separation between storage and inference layers
