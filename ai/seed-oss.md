# Seed-OSS

![logo](logo)

Seed-OSS is a series of open-source large language models developed by ByteDance's Seed Team, designed for powerful long-context, reasoning, agent and general capabilities, and versatile developer-friendly features. Although trained with only 12T tokens, Seed-OSS achieves excellent performance on several popular open benchmarks.
Powered by Unsloth's GGUF conversion.

## Intended uses

- **Conversational AI**: Engaging in dialogue with users, providing informative and contextually relevant responses.
- **Reasoning tasks**: Excelling in logical reasoning and problem-solving scenarios.
- **Multi-agent frameworks**: Facilitating interactions between multiple AI agents for complex tasks.

## Characteristics

| Attribute        | Details    |
|------------------|------------|
| **Provider**     | ByteDance  |
| **Architecture** | seed_oss   |
| **Cutoff date**  | July 2024  |
| **Tool calling** | ✅          |
| **License**      | Apache 2.0 |

## Available model variants

| Model variant                                            | Parameters | Quantization  | Context window | VRAM¹     | Size     |
|----------------------------------------------------------|------------|---------------|----------------|-----------|----------|
| `ai/seed-oss:latest`<br><br>`ai/seed-oss:36B-UD-Q4_K_XL` | 36B        | MOSTLY_Q4_K_M | 524K tokens    | 22.38 GiB | 20.51 GB |
| `ai/seed-oss:36B-UD-IQ1_M`                               | 36B        | MOSTLY_IQ1_M  | 524K tokens    | 10.42 GiB | 8.45 GB  |
| `ai/seed-oss:36B-UD-Q4_K_XL`                             | 36B        | MOSTLY_Q4_K_M | 524K tokens    | 22.38 GiB | 20.51 GB |
| `ai/seed-oss:36B-UD-Q6_K_XL`                             | 36B        | MOSTLY_Q6_K   | 524K tokens    | 31.15 GiB | 29.66 GB |

¹: VRAM estimated based on model characteristics.

> `latest` → `36B-UD-Q4_K_XL`

## Use this AI model with Docker Model Runner

```bash
docker model run ai/seed-oss
```

## Links
- https://seed.bytedance.com/en/
- https://huggingface.co/unsloth/Seed-OSS-36B-Instruct-GGUF
