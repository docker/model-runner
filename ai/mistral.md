# Mistral 7B Instruct v0.2

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/mistral-280x184-overview@2x.svg)

A fast and powerful 7B parameter model excelling in reasoning, code, and math.
Mistral 7B is a powerful 7.3B parameter language model that outperforms Llama 2 13B across a wide range of benchmarks, including reasoning, reading comprehension, and code generation. Despite its smaller size, it delivers performance comparable to much larger models, making it efficient and versatile.

## Intended uses

Mistral 7B is designed to provide high-quality responses across a wide range of general-purpose NLP tasks while remaining efficient in resource usage.
Also, this model is fine-tuned to follow instructions, allowing it to perform tasks and answer questions naturally. The base model doesn’t have this capability.

- **Automated code generation:** Automates creation of code snippets, reducing manual coding and accelerating development.
- **Debugging support:** Identifies code errors and provides actionable recommendations to streamline debugging.
- **Text summarization and classification:** Supports summarizing text, classification, and text/code completion tasks.
- **Conversational applications:** Fine-tuned for conversational interactions using diverse datasets.
- **Knowledge retrieval:** Delivers accurate, detailed answers for enhanced information retrieval.
- **Mathematical accuracy:** Reliably processes and solves complex mathematical problems.
- **Roleplay and text generation:** Generates extensive narrative text for roleplaying and creative scenarios.

## Characteristics

| Attribute             | Details                          |
|-----------------------|----------------------------------|
| **Provider**          | Mistral AI                       |
| **Architecture**      | Llama                            |
| **Cutoff date**       | December 2023ⁱ                   |
| **Languages**         | English (primarily)              |
| **Tool calling**      | ❌                               |
| **Input modalities**  | Text                             |
| **Output modalities** | Text                             |
| **License**           | Apache 2.0                       |

i: Estimated

## Available model variants

| Model variant | Parameters | Quantization | Context window | VRAM¹ | Size |
|---------------|------------|--------------|----------------|------|-------|
| `ai/mistral:latest`<br><br>`ai/mistral:7B-Q4_K_M` | 7B | IQ2_XXS/Q4_K_M | 33K tokens | 2.02 GB | 4.07 GB |
| `ai/mistral:7B-Q4_0` | 7B | Q4_0 | 33K tokens | 4.40 GB | 3.83 GB |
| `ai/mistral:7B-Q4_K_M` | 7B | IQ2_XXS/Q4_K_M | 33K tokens | 2.02 GB | 4.07 GB |
| `ai/mistral:7B-F16` | 7B | F16 | 33K tokens | 15.65 GB | 13.50 GB |

¹: VRAM estimated based on model characteristics.

> `latest` → `7B-Q4_K_M`

## Use this AI model with Docker Model Runner

First, pull the model:

```bash
docker model pull ai/mistral
```

Then run the model:

```bash
docker model run ai/mistral
```

For more information on Docker Model Runner, [explore the documentation](https://docs.docker.com/desktop/features/model-runner/).

## Considerations

- Best suited for English.
- Performs well out-of-the-box but can be fine-tuned further.
- Use appropriate system prompts for safer and more controlled outputs.
- To use instruction fine-tuning, wrap your prompt with `[INST]` and `[/INST]` tags. The first instruction must start with a beginning-of-sentence token, while any following instructions should not. The assistant's response will automatically end with an end-of-sentence token. 

## Benchmark performance

| Capability                     | Benchmark  | Mistral 7B |
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

- [Mistral 7b](https://mistral.ai/news/announcing-mistral-7b)
- [Mistral 7b-Paper](https://arxiv.org/abs/2310.06825)
