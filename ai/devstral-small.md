# Devstral Small 1.1
*GGUF version by Unsloth*

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/mistral-280x184-overview@2x.svg)

## Description
Devstral Small 1.1 is an agentic coding LLM (24B) fine-tuned from Mistral-Small-3.1 with a 128K context window. It’s designed for software engineering agents and supports Mistral-style tool/function calling. Text-only. Apache-2.0 on this Unsloth GGUF release.

## Characteristics

| Attribute             | Details                                                   |
|-----------------------|-----------------------------------------------------------|
| **Provider**          | Mistral AI                                                |
| **Architecture**      | Llama                                                     |
| **Cutoff date**       | 2023-10-01                                                |
| **Languages**         | Multilingual (24 languages)                               |
| **Tool calling**      | ✅                                                         |
| **Input modalities**  | Text                                                      |
| **Output modalities** | Text                                                      |
| **License**           | [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) |

## Available model variants

| Model variant | Parameters | Quantization | Context window | VRAM¹ | Size |
|---------------|------------|--------------|----------------|------|-------|
| `ai/devstral-small:24B`<br><br>`ai/devstral-small:24B-UD-Q4_K_XL`<br><br>`ai/devstral-small:latest` | 23.57 B | MOSTLY_Q4_K_M | 131K tokens | 14.63 GiB | 13.54 GB |

¹: VRAM estimated based on model characteristics.

> `latest` → `24B`

## Use this AI model with Docker Model Runner

```bash
docker model run ai/devstral-small
```

## Considerations

- Text-only: the underlying vision encoder was removed before fine-tuning. Don’t expect image inputs to work.

## Benchmark performance

| Category      | Metric                                  | Devstral Small 1.1 |
|---------------|-----------------------------------------|--------------------|
| **SWE-Bench** |                                         |                    |
|               | SWE-Bench Verified (OpenHands scaffold) | 53.6%              |

## Links
- [Mistral AI Announcement](https://mistral.ai/news/devstral-2507)
- [Hugging Face GGUF](https://huggingface.co/unsloth/Devstral-Small-2507-GGUF)
- [Unsloth Dynamic 2.0 GGUF](https://docs.unsloth.ai/basics/unsloth-dynamic-2.0-ggufs)
