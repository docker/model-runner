# GPT‑OSS

Welcome to the gpt-oss series, OpenAI’s open-weight models designed for powerful reasoning, agentic tasks, and versatile developer use cases.

## Available model variants

| Model variant | Parameters | Quantization | Context window | VRAM¹ | Size |
|---------------|------------|--------------|----------------|------|-------|
| `ai/gpt-oss:latest`<br><br>`ai/gpt-oss:20B-UD-Q4_K_XL` | 20B | MOSTLY_Q4_K_M | 131K tokens | 11.97 GiB | 11.04 GB |
| `ai/gpt-oss:20B-F16` | 20B | MOSTLY_F16 | 131K tokens | 13.25 GiB | 12.83 GB |
| `ai/gpt-oss:20B-UD-Q4_K_XL` | 20B | MOSTLY_Q4_K_M | 131K tokens | 11.97 GiB | 11.04 GB |
| `ai/gpt-oss:20B-UD-Q6_K_XL` | 20B | MOSTLY_Q6_K | 131K tokens | 12.12 GiB | 11.20 GB |
| `ai/gpt-oss:20B-UD-Q8_K_XL` | 20B | MOSTLY_Q8_0 | 131K tokens | 12.69 GiB | 12.28 GB |

¹: VRAM estimated based on model characteristics.

> `latest` → `20B-UD-Q4_K_XL`

## Use this AI model with Docker Model Runner

Run the model:

```bash
docker model run ai/gpt-oss
```

## Considerations

- Please note that response parsing issues are still present in the current version
- CoT and tool calling are not supported

## Links
- https://huggingface.co/openai/gpt-oss-20b
- https://openai.com/index/introducing-gpt-oss/
