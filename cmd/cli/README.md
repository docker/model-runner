# Docker Model CLI

A powerful command-line interface for managing, running, packaging, and deploying AI/ML models using Docker. This CLI lets you install and control the Docker Model Runner, interact with models, manage model artifacts, and integrate with OpenAI and other backends—all from your terminal.

## Features
- **Install Model Runner**: Easily set up the Docker Model Runner for local or cloud environments with GPU support.
- **Run Models**: Execute models with prompts or in interactive chat mode, supporting multiline input and OpenAI-style backends.
- **List Models**: View all models available locally or via OpenAI, with options for JSON and quiet output.
- **Package Models**: Convert GGUF files into Docker model OCI artifacts and push them to registries, including license and context size options.
- **Configure Models**: Set runtime flags and context sizes for models.
- **Logs & Status**: Stream logs and check the status of the Model Runner and individual models.
- **Tag, Pull, Push, Remove, Unload**: Full lifecycle management for model artifacts.
- **Compose & Desktop Integration**: Advanced orchestration and desktop support for model backends.

## Building
1. **Clone the repo:**
   ```bash
   git clone https://github.com/docker/model-cli.git
   cd model-cli
   ```
2. **Build the CLI:**
   ```bash
   make build
   ```
3. **Install Model Runner:**
   ```bash
   ./model-cli install-runner
   ```
   Use `--gpu cuda` for GPU support, or `--gpu auto` for automatic detection.

## Usage
Run `./model-cli --help` to see all commands and options.

### Common Commands
- `model-cli install-runner` — Install the Docker Model Runner
- `model-cli start-runner` — Start the Docker Model Runner
- `model-cli stop-runner` — Stop the Docker Model Runner
- `model-cli restart-runner` — Restart the Docker Model Runner
- `model-cli run MODEL [PROMPT]` — Run a model with a prompt or enter chat mode
- `model-cli list` — List available models
- `model-cli package --gguf <path> --push <target>` — Package and push a model
- `model-cli logs` — View logs
- `model-cli status` — Check runner status
- `model-cli configure MODEL [flags]` — Configure model runtime
- `model-cli unload MODEL` — Unload a model
- `model-cli tag SOURCE TARGET` — Tag a model
- `model-cli pull MODEL` — Pull a model
- `model-cli push MODEL` — Push a model
- `model-cli rm MODEL` — Remove a model

## Example: Interactive Chat
```bash
./model-cli run llama.cpp "What is the capital of France?"
```
Or enter chat mode:
```bash
./model-cli run llama.cpp
> """
Tell me a joke.
"""
```

## Advanced
- **Packaging:**
  Add licenses and set context size when packaging models for distribution.

## Development
- **Run unit tests:**
  ```bash
  make unit-tests
  ```
- **Generate docs:**
  ```bash
  make docs
  ```

## License
[Apache 2.0](LICENSE)

