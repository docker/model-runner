# Model Distribution

A Go library for distributing models using container registries.

## Overview

Model Distribution is a Go library that allows you to package, push, pull, and manage models using container registries. It provides a simple API for working with models in GGUF and Safetensors format.

## Features

- Push models to container registries
- Pull models from container registries
- Local model storage
- Model metadata management
- Support for both GGUF and Safetensors model formats

## Usage

```go
import (
    "context"
    "github.com/docker/model-runner/pkg/distribution/distribution"
)

// Create a new client
client, err := distribution.NewClient(
    distribution.WithStoreRootPath("/path/to/cache"),
)
if err != nil {
    // Handle error
}

// Pull a model
err = client.PullModel(context.Background(), "registry.example.com/models/llama:v1.0", os.Stdout)
if err != nil {
    // Handle error
}

// Get a model
model, err := client.GetModel("registry.example.com/models/llama:v1.0")
if err != nil {
    // Handle error
}

// Create a bundle
bundle, err := client.GetBundle("registry.example.com/models/llama:v1.0")
if err != nil {
    // Handle error
}

// Get the GGUF file path within the bundle
modelPath, err := bundle.GGUFPath()
if err != nil {
    // Handle error
}

fmt.Println("Model path:", modelPath)

// List all models
models, err := client.ListModels()
if err != nil {
    // Handle error
}

// Delete a model
_, err = client.DeleteModel("registry.example.com/models/llama:v1.0", false)
if err != nil {
    // Handle error
}

// Tag a model
err = client.Tag("registry.example.com/models/llama:v1.0", "registry.example.com/models/llama:latest")
if err != nil {
    // Handle error
}

// Push a model
err = client.PushModel(context.Background(), "registry.example.com/models/llama:v1.0", nil)
if err != nil {
    // Handle error
}
```