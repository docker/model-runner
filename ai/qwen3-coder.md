# Qwen3‑Coder‑30B‑A3B‑Instruct

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/qwen-280x184-overview@2x.svg)

Open‑source agentic coding model optimized for long‑context, instruction‑following code generation and tooling.

## Intended uses

Lightweight yet powerful coding assistant for repository‑scale tasks:

- **Agentic coding workflows**: Automate multi‑step code tasks with platform integrations (e.g. Qwen Code, CLINE).
- **Browser‑use scenarios**: Drive agentic browser interactions for search, scraping, or UI automation.
- **Large code context comprehension**: Handle repository‑scale files with native support up to 256K token context (extendable to ~1M via Yarn).

## Characteristics

| Attribute             | Details        |
|----------------------|----------------|
| **Provider**          | Qwen / Alibaba |
| **Architecture**      | MoE (Mixture of Experts, 30.5B total with ~3.3B active, 128 experts, 8 active) |
| **Cutoff date**       | July 2025 (model released August 2025) |
| **Languages**         | Multilingual; over 100 spoken and programming languages |
| **Tool calling**      | Yes |
| **Input modalities**  | Text (code + natural language) |
| **Output modalities** | Text (code + natural language) |
| **License**           | Apache‑2.0 |

## Available model variants

| Model variant                                 | Parameters | Quantization     | Context window    | VRAM¹   | Size       |
|----------------------------------------------|------------|------------------|-------------------|---------|------------|
| `ai/qwen3-coder:30B-A3B-UD-Q4_K_XL`          | 30 B-A3B   | MOSTLY_Q4_K_M    | 262K tokens       | ~17.2GiB| ~16.45GB   |

¹VRAM estimated for quantized model type.

## Use this AI model with Docker Model Runner

First, pull the model:

```bash
docker model pull ai/qwen3-coder:30B-A3B-UD-Q4_K_XL
```

Then run the model:

```bash
docker model run ai/qwen3-coder:30B-A3B-UD-Q4_K_XL
```

For more information on Docker Model Runner, [explore the documentation](https://docs.docker.com/desktop/features/model-runner/).

## Links

- [Hugging Face](https://huggingface.co/unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF)
