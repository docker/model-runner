# Mistral Nemo Instruct 2407

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/mistral-280x184-overview@2x.svg)

Mistral-Nemo-Instruct-2407 is an instruct fine-tuned large language model developed by Mistral AI and NVIDIA, optimized for multilingual tasks and instruction-following capabilities.
Compared to Mistral 7B, it is much better at following precise instructions, reasoning, handling multi-turn conversations, and generating code.

## Intended Uses

Mistral-Nemo-Instruct-2407 is designed for instruction-following tasks and multilingual applications, including:

- **Conversational AI**: Developing interactive, multilingual chatbots.
- **Knowledge retrieval**: Answering questions across multiple languages.
- **Code assistance**: Generating and understanding code snippets.

## Characteristics

| Attribute             | Details                                                                                 |
|-----------------------|-----------------------------------------------------------------------------------------|
| **Provider**          | Mistral AI & NVIDIA                                                                     |
| **Architecture**      | llama                                                                                   |
| **Cutoff date**       | July 2024                                                                               |
| **Languages**         | English, French, German, Spanish, Italian, Portuguese, Russian, Chinese, Japanese       |
| **Tool calling**      | ✅                                                                                      |
| **Input modalities**  | Text                                                                                    |
| **Output modalities** | Text                                                                                    |
| **License**           | Apache 2.0                                                                              |

## Available model variants

| Model variant | Parameters | Quantization | Context window | VRAM¹ | Size |
|---------------|------------|--------------|----------------|------|-------|
| `ai/mistral-nemo:latest`<br><br>`ai/mistral-nemo:12B-Q4_K_M` | 12B | IQ2_XXS/Q4_K_M | 131K tokens | 7.78 GiB | 6.96 GB |
| `ai/mistral-nemo:12B-Q4_K_M` | 12B | IQ2_XXS/Q4_K_M | 131K tokens | 7.78 GiB | 6.96 GB |

¹: VRAM estimated based on model characteristics.

> `latest` → `12B-Q4_K_M`

## Use this AI model with Docker Model Runner

First, pull the model:

```bash
docker model pull ai/mistral-nemo
```

Then run the model:

```bash
docker model run ai/mistral-nemo
```

For more information on Docker Model Runner, [explore the documentation](https://docs.docker.com/desktop/features/model-runner/).

## Benchmark performance

| Benchmark                   | Score |
|-----------------------------|-------|
| HellaSwag (0-shot)          | 83.5% |
| Winogrande (0-shot)         | 76.8% |
| OpenBookQA (0-shot)         | 60.6% |
| CommonSenseQA (0-shot)      | 70.4% |
| TruthfulQA (0-shot)         | 50.3% |
| MMLU (5-shot)               | 68.0% |
| TriviaQA (5-shot)           | 73.8% |
| NaturalQuestions (5-shot)   | 31.2% |

## Links

- [Mistral Nemo](https://mistral.ai/news/mistral-nemo)
