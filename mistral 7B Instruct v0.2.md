# Mistral 7B Instruct v0.2

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/mistral-280x184-overview@2x.svg)

**A fast and powerful 7B parameter model excelling in reasoning, code, and math.**
Mistral 7B is a powerful 7.3B parameter language model that outperforms Llama 2 13B across a wide range of benchmarks, including reasoning, reading comprehension, and code generation. Despite its smaller size, it delivers performance comparable to much larger models, making it efficient and versatile.

## Characteristics

| Attribute             | Details                          |
|-----------------------|----------------------------------|
| **Provider**          | Mistral AI                       |
| **Architecture**      | Llama                            |
| **Cutoff Date**       | December 2023ⁱ                   |
| **Languages**         | English (primarily)              |
| **Tool calling**      | ❌                               |
| **Input Modalities**  | Text                             |
| **Output Modalities** | Text                             |
| **License**           | Apache 2.0                       |

i: Estimated

## Available Model Variants

| Model Variant                                      | Parameters | Quantization   | Context Window | VRAM    | Size   | 
|----------------------------------------------------|----------- |--------------- |----------------|---------|--------|
| `ai/mistral:latest`<br><br>`ai/mistral:7B-Q4_K_M`  | 7B         | IQ2_XXS/Q4_K_M | 32K            | 4.2B¹   | 4.3GB  | 
| `ai/mistral:7B-F16`                                | 7B         | F16            | 32K            | 16.8¹   | 14.5GB |

¹: VRAM estimated based on model characteristics and quantization.

> `:latest` → `7B-Q4_K_M` 


## Intended Uses

Mistral 7B is designed to provide high-quality responses across a wide range of general-purpose NLP tasks while remaining efficient in resource usage.
Also, this model is fine-tuned to follow instructions, allowing it to perform tasks and answer questions naturally. The base model doesn’t have this capability.

- **Automated Code Generation:** Automates creation of code snippets, reducing manual coding and accelerating development.
- **Debugging Support:** Identifies code errors and provides actionable recommendations to streamline debugging.
- **Text Summarization and Classification:** Supports summarizing text, classification, and text/code completion tasks.
- **Conversational Applications:** Fine-tuned for conversational interactions using diverse datasets.
- **Knowledge Retrieval:** Delivers accurate, detailed answers for enhanced information retrieval.
- **Mathematical Accuracy:** Reliably processes and solves complex mathematical problems.
- **Roleplay and Text Generation:** Generates extensive narrative text for roleplaying and creative scenarios.

## Considerations

- Best suited for English.
- Performs well out-of-the-box but can be fine-tuned further.
- Use appropriate system prompts for safer and more controlled outputs.
- To use instruction fine-tuning, wrap your prompt with `[INST]` and `[/INST]` tags. The first instruction must start with a beginning-of-sentence token, while any following instructions should not. The assistant's response will automatically end with an end-of-sentence token. 

## How to Run This AI Model

You can pull the model using:

```
docker model pull ai/mistral
```

Run this model using:

```
docker model run ai/mistral
```

## Benchmark Performance


| Category                       | Benchmark  | Mistral 7B |
|--------------------------------|------------|------------|
| Natural Language Understanding | MMLU       | 60.1%      |
|                                | HellaSwag  | 81.3%      |
|                                | WinoGrande | 75.3%      |
|                                | PIQA       | 83.0%      |
|                                | Arc-e      | 80.0%      |
|                                | Arc-c      | 55.5%      |
| Knowledge Retrieval            | NQ         | 28.8%      |
|                                | TriviaQA   | 69.9%      |
| Code Generation & Debugging    | HumanEval  | 30.5%      |
|                                | MBPP       | 47.5%      |
| Mathematical Reasoning         | MATH       | 13.1%      |
|                                | GSM8K      | 52.1%      |

## Links
* [Mistral 7b](https://mistral.ai/news/announcing-mistral-7b)
* [Mistral 7b-Paper](https://arxiv.org/abs/2310.06825)
