
# Qwen2.5-7B Instruct

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/qwen-280x184-overview@2x.svg)

Qwen2.5-7B-Instruct is an instruction-tuned large language model developed by Alibaba Cloud. It is part of the Qwen2.5 series, which includes models ranging from 0.5 to 72 billion parameters. This model offers significant improvements in knowledge, coding, and mathematical capabilities, along with enhanced instruction-following and long-text generation abilities. It supports a context length of up to 131,072 tokens and can generate outputs up to 8,192 tokens. Additionally, it provides multilingual support for over 29 languages, including Chinese, English, French, Spanish, Portuguese, German, Italian, Russian, Japanese, Korean, Vietnamese, Thai, and Arabic.

## Intended uses

Qwen2.5-7B-Instruct is designed to assist in various natural language processing tasks, including:

- **Conversational AI**: Engaging in dialogue with users, providing informative and contextually relevant responses.
- **Text generation**: Creating coherent and contextually appropriate text based on prompts.
- **Multilingual support**: Understanding and generating text in multiple languages, facilitating cross-lingual communication.
- **Structured data understanding**: Working with tables, JSON, and semi-structured input/output


## Characteristics

| Attribute             | Details            |
|---------------------- |--------------------|
| **Provider**          | Alibaba Cloud      |
| **Architecture**      | qwen2              |
| **Cutoff date**       | November 2024 (est)|
| **Languages**         | Chinese, English, French, Spanish, Portuguese, German, Italian, Russian, Japanese, Korean, Vietnamese, Thai, Arabic, and more (29 languages) |
| **Tool calling**      | ✅                 |
| **Input modalities**  | Text               |
| **Output modalities** | Text               |
| **License**           | Apache 2.0         |

## Available model variants

| Model variant | Parameters | Quantization | Context window | VRAM¹ | Size |
|---------------|------------|--------------|----------------|------|-------|
| `ai/qwen2.5:latest`<br><br>`ai/qwen2.5:7B-Q4_K_M` | 7B | IQ2_XXS/Q4_K_M | 33K tokens | 2.32 GB | 4.36 GB |
| `ai/qwen2.5:0.5B-F16` | 0.5B | F16 | 33K tokens | 4.27 GB | 942.43 MB |
| `ai/qwen2.5:1.5B-F16` | 1.5B | F16 | 33K tokens | 4.85 GB | 2.88 GB |
| `ai/qwen2.5:3B-Q4_K_M` | 3B | IQ2_XXS/Q4_K_M | 33K tokens | 2.06 GB | 1.79 GB |
| `ai/qwen2.5:3B-F16` | 3B | F16 | 33K tokens | 7.91 GB | 5.75 GB |
| `ai/qwen2.5:7B-Q4_0` | 7B | Q4_0 | 33K tokens | 4.70 GB | 4.12 GB |
| `ai/qwen2.5:7B-Q4_K_M` | 7B | IQ2_XXS/Q4_K_M | 33K tokens | 2.32 GB | 4.36 GB |
| `ai/qwen2.5:7B-F16` | 7B | F16 | 33K tokens | 15.95 GB | 14.19 GB |

¹: VRAM estimated based on model characteristics.

> `latest` → `7B-Q4_K_M`

## Use this AI model with Docker Model Runner

First, pull the model:

```bash
docker model pull ai/qwen2.5
```

Then run the model:

```bash
docker model run ai/qwen2.5
```

For more information on Docker Model Runner, [explore the documentation](https://docs.docker.com/desktop/features/model-runner/).

## Considerations

- Ensure that the model is used in accordance with its Apache 2.0 license.
- Be mindful of the computational resources required, especially when handling long-context inputs.
- Regularly update to the latest version to benefit from improvements and security updates.

# Benchmark performance

| Metrics                   | Benchmark                | Qwen2.5-7B-Instruct |
|---------------------------|--------------------------|---------------------|
| Knowledge & QA            | MMLU-Pro                 | 56.3                |
|                           | MMLU-redux               | 75.4                |
|                           | GPQA                     | 36.4                |
| Math & Reasoning          | MATH                     | 75.5                |
|                           | GSM8K                    | 91.6                |
| Code                      | HumanEval                | 84.8                |
|                           | MBPP                     | 79.2                |
|                           | MultiPL-E                | 70.4                |
|                           | LiveCodeBench 2305-2409  | 28.7                |
|                           | LiveBench 0831           | 35.9                |
| Instruction Following     | IFeval strict-prompt     | 71.2                |
|                           | Arena-Hard               | 52.0                |
| Alignment & Preference    | AlignBench v1.1          | 7.33                |
|                           | MTbench                  | 8.75                |

## Links

- [Qwen2.5: A Party of Foundation Models](https://qwenlm.github.io/blog/qwen2.5/)
- [Qwen2.5 Technical Report](https://arxiv.org/abs/2412.15115)
