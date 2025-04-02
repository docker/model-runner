
# Qwen2.5-7B Instruct

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/qwen-280x184-overview@2x.svg)

Qwen2.5-7B-Instruct is an instruction-tuned large language model developed by Alibaba Cloud. It is part of the Qwen2.5 series, which includes models ranging from 0.5 to 72 billion parameters. This model offers significant improvements in knowledge, coding, and mathematical capabilities, along with enhanced instruction-following and long-text generation abilities. It supports a context length of up to 131,072 tokens and can generate outputs up to 8,192 tokens. Additionally, it provides multilingual support for over 29 languages, including Chinese, English, French, Spanish, Portuguese, German, Italian, Russian, Japanese, Korean, Vietnamese, Thai, and Arabic.

## Characteristics

| Attribute             | Details            |
|---------------------- |--------------------|
| **Provider**          | Alibaba Cloud      |
| **Architecture**      | qwen2              |
| **Cutoff Date**       | November 2024 (est)|
| **Languages**         | Chinese, English, French, Spanish, Portuguese, German, Italian, Russian, Japanese, Korean, Vietnamese, Thai, Arabic, and more (29 languages) |
| **Tool Calling**      | ✅                 |
| **Input Modalities**  | Text               |
| **Output Modalities** | Text               |
| **License**           | Apache 2.0         |

## Available Model Variants

| Model Variant                        | Parameters | Quantization       | Context Window | VRAM      | Size    |                  
|--------------------------------------|------------|--------------------|----------------|-----------|---------|
| `qwen2.5:7B-F16`                     | 7.61B      | F16                | 128k tokens    | 18GB¹     | 14.2GB  | 
| `qwen2.5:latest` `qwen2.5:7B-Q4_K_M` | 7.61B      | IQ2_XXS/Q4_K_MGGUF | 128k tokens    | 5GB¹      | 4.4GB   | 

¹: VRAM estimates based on model characteristics.

`:latest`→ `qwen2.5:7B-Q4_K_M`

## Intended Uses

Qwen2.5-7B-Instruct is designed to assist in various natural language processing tasks, including:

- **Conversational AI**: Engaging in dialogue with users, providing informative and contextually relevant responses.
- **Text Generation**: Creating coherent and contextually appropriate text based on prompts.
- **Multilingual Support**: Understanding and generating text in multiple languages, facilitating cross-lingual communication.
- **Structured Data Understanding**: Working with tables, JSON, and semi-structured input/output

## Considerations

- Ensure that the model is used in accordance with its Apache 2.0 license.
- Be mindful of the computational resources required, especially when handling long-context inputs.
- Regularly update to the latest version to benefit from improvements and security updates.

## How to Run This AI Model

You can pull the model using:

```
docker model pull Qwen2.5-7B-Instruct
```

Run this model using:

```
docker model run Qwen2.5-7B-Instruct
```

# Benchmark and Performance
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
