# Model Cards CLI

A command-line tool for working with model cards. It can update the "Available model variants" tables in model card markdown files and inspect model repositories to extract metadata.

## Features

- Scans the `ai/` directory for markdown files
- For each model, fetches OCI manifest information
- Locates GGUF files in the manifest via mediaType
- Extracts metadata from GGUF files without downloading the entire file
- Updates the "Available model variants" table in each markdown file

## Installation

```bash
go mod tidy
make build
```

## Usage

The Model Cards CLI provides three main commands:

1. `update` - Updates the "Available model variants" tables in model card markdown files
2. `inspect-model` - Inspects a model repository and displays metadata about the model variants
3. `upload-overview` - Uploads an overview to Docker Hub for a specified repository

### Update Command

You can use the provided Makefile to build and run the application:

```bash
# Build the Go application
make build

# Update all model files
./bin/model-cards-cli update

# Update a specific model file
./bin/model-cards-cli update --model-file=<model-file.md>
```

By default, the tool will scan all markdown files in the `ai/` directory and update their "Available model variants" tables. If you specify a model file with the `--model-file` flag or the `MODEL` parameter, it will only update that specific file.

To override the source namespace used to build the repository name, pass the `--namespace` flag:

Examples:
- `./bin/model-cards-cli update --namespace=myorg`
- `./bin/model-cards-cli update --model-file=llama3.1.md --namespace=myorg`

#### Update Command Options

- `--model-dir`: Directory containing model markdown files (default: "../../ai")
- `--model-file`: Specific model markdown file to update (without path)
- `--namespace`: Namespace to use for repositories (overrides deriving from file path; e.g., "--namespace=myorg" makes repo "myorg/file-basename")
- `--log-level`: Log level (debug, info, warn, error) (default: "info")
