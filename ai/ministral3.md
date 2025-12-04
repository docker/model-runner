# Ministral 3 Instruct 2512
*GGUF version by Unsloth*

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/mistral-280x184-overview@2x.svg)

## Description
The Ministral 3 family consists of compact, efficient multimodal language models designed for edge deployment and local inference. All three variants—3B, 8B, and 14B—offer strong instruction-following capabilities, vision support, and broad hardware compatibility. Each model is released in GGUF format across multiple quantization levels, enabling flexible trade-offs between performance and resource usage.
All variants are post-trained for instruction tasks, making them ideal for:
- Chat-based applications
- Assistants and agents
- On-device inference
- CPU and GPU constrained environments
- Multimodal (vision + text) use cases


## Characteristics

| Attribute             | Details                                                                                                                                  |
|-----------------------|------------------------------------------------------------------------------------------------------------------------------------------|
| **Provider**          | Mistral AI                                                                                                                               |
| **Architecture**      | mistral3                                                                                                                                 |
| **Languages**         | Supports dozens of languages, including English, French, Spanish, German, Italian, Portuguese, Dutch, Chinese, Japanese, Korean, Arabic. |
| **Tool calling**      | ✅                                                                                                                                        |
| **Input modalities**  | Text, Images                                                                                                                             |
| **Output modalities** | Text                                                                                                                                     |
| **License**           | Apache 2.0                                                                                                                               |

## Available model variants

| Model variant                                                                     | Parameters | Quantization  | Context window | VRAM¹     | Size     |
|-----------------------------------------------------------------------------------|------------|---------------|----------------|-----------|----------|
| `ai/ministral3:8B`<br><br>`ai/ministral3:8B-Q4_K_M`<br><br>`ai/ministral3:latest` | 8B         | MOSTLY_Q4_K_M | 262K tokens    | 5.89 GiB  | 4.83 GB  |
| `ai/ministral3:14B`                                                               | 14B        | MOSTLY_Q4_K_M | 262K tokens    | 8.87 GiB  | 7.78 GB  |
| `ai/ministral3:14B-BF16`                                                          | 14B        | MOSTLY_BF16   | 262K tokens    | 25.35 GiB | 25.16 GB |
| `ai/ministral3:14B-UD-Q8_K_XL`                                                    | 14B        | MOSTLY_Q8_0   | 262K tokens    | 16.13 GiB | 15.93 GB |
| `ai/ministral3:8B-BF16`                                                           | 8B         | MOSTLY_BF16   | 262K tokens    | 16.15 GiB | 15.81 GB |
| `ai/ministral3:3B-Q4_K_M`                                                         | 3B         | MOSTLY_Q4_K_M | 262K tokens    | 3.19 GiB  | 1.99 GB  |
| `ai/ministral3:3B-BF16`                                                           | 3B         | MOSTLY_BF16   | 262K tokens    | 7.59 GiB  | 6.39 GB  |

¹: VRAM estimated based on model characteristics.

> `latest` → `8B`

## Use this AI model with Docker Model Runner

```bash
docker model run ai/ministral3
```

## Use cases

Private AI deployments where advanced capabilities meet practical hardware constraints:

- Private/custom chat and AI assistant deployments in constrained environments
- Advanced local agentic use cases
- Fine-tuning and specialization
- And more...
Bringing advanced AI capabilities to most environments.

## Links
- [Hugging Face (Mistral AI)](https://huggingface.co/mistralai/Ministral-3-14B-Instruct-2512)
- [Hugging Face (Unsloth GGUF)](https://huggingface.co/unsloth/Ministral-3-14B-Instruct-2512-GGUF)
- [Unsloth Dynamic 2.0 GGUF](https://docs.unsloth.ai/basics/unsloth-dynamic-2.0-ggufs)
