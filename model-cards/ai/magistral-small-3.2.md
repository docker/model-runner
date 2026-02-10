# Mistral-Small-3.2-24B-Instruct-2506
*GGUF version by Unsloth*

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/mistral-280x184-overview@2x.svg)

## Description
A 24B-parameter instruction-tuned multimodal model by Mistral AI, optimized for following instructions, reducing repetition errors, and supporting function/tool calling.

## Characteristics

| Attribute             | Details      |
|-----------------------|--------------|
| **Provider**          | Mistral AI   |
| **Architecture**      | Llama        |
| **Cutoff date**       | 2023-10-01   |
| **Languages**         | 24 languages |
| **Tool calling**      | ✅            |
| **Input modalities**  | Text, Images |
| **Output modalities** | Text         |
| **License**           | Apache 2.0   |

## Available model variants

| Model variant | Parameters | Quantization | Context window | VRAM¹ | Size |
|---------------|------------|--------------|----------------|------|-------|
| `ai/magistral-small-3.2:24B`<br><br>`ai/magistral-small-3.2:24B-UD-Q4_K_XL`<br><br>`ai/magistral-small-3.2:latest` | 23.57 B | MOSTLY_Q4_K_M | 41K tokens | 14.59 GiB | 13.50 GB |
| `ai/magistral-small-3.2:24B-UD-IQ2_XXS` | 23.57 B | MOSTLY_IQ2_XXS | 41K tokens | 7.45 GiB | 6.28 GB |

¹: VRAM estimated based on model characteristics.

> `latest` → `24B`

## Use this AI model with Docker Model Runner

```bash
docker model run ai/magistral-small-2506
```

## Considerations

- We recommend using a relatively low temperature, such as temperature=0.15.
- Make sure to add a system prompt to the model to best tailor it for your needs. If you want to use the model as a general assistant, we recommend to use the one provided in the SYSTEM_PROMPT.txt file.

## Benchmark performance

| Category                  | Metric                    | Mistral-Small-3.2-24B-Instruct-2506 |
|---------------------------|---------------------------|-------------------------------------|
| **Instruction Following** | WildBench v2              | 65.33%                              |
|                           | Arena Hard v2             | 43.1%                               |
|                           | IF Accuracy (Internal)    | 84.78%                              |
| **STEM**                  | MMLU                      | 80.50%                              |
|                           | MMLU Pro (5-shot CoT)     | 69.06%                              |
|                           | MATH                      | 69.42%                              |
|                           | GPQA Diamond (5-shot CoT) | 46.13%                              |
|                           | MBPP Plus Pass@5          | 78.33%                              |
|                           | HumanEval Plus Pass@5     | 92.90%                              |
|                           | SimpleQA (TotalAcc)       | 12.10%                              |
| **Vision**                | MMMU                      | 62.5%                               |
|                           | ChartQA                   | 87.4%                               |
|                           | DocVQA                    | 94.86%                              |
|                           | AI2D                      | 92.91%                              |


## Links
- [Hugging Face (Mistral AI)](https://huggingface.co/mistralai/Mistral-Small-3.2-24B-Instruct-2506)
- [Hugging Face (Unsloth GGUF)](https://huggingface.co/unsloth/Mistral-Small-3.2-24B-Instruct-2506-GGUF)
- [Unsloth Dynamic 2.0 GGUF](https://docs.unsloth.ai/basics/unsloth-dynamic-2.0-ggufs)
