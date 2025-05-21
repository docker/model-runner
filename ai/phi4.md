# Phi-4 

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/phi-280x184-overview@2x.svg)

Phi-4 is a **14-billion parameter** language model developed by Microsoft Research. It is part of the Phi model family, emphasizing **data quality and reasoning capabilities** over pure scale.

## Intended uses

Phi-4 is designed for:
- **STEM reasoning and problem-solving** (math, physics, coding, logic)
- **Memory and compute-efficient applications**
- **Educational and research applications**
- **Conversational AI and chatbot experiences**

## Characteristics

| Attribute             | Details       |
|---------------------- |---------------|
| **Provider**          | Microsoft     |
| **Architecture**      | phi3          |
| **Cutoff date**       | June 2024     |
| **Languages**         | English (primary), German, Spanish, French, Portuguese, Italian, Hindi, Japanese |
| **Tool calling**      | ❌            |
| **Input modalities**  | Text          |
| **Output modalities** | Text          |
| **License**           | MIT           |

## Available model variants

| Model variant | Parameters | Quantization | Context window | VRAM¹ | Size |
|---------------|------------|--------------|----------------|------|-------|
| `ai/phi4:latest`<br><br>`ai/phi4:14B-Q4_K_M` | 15B | IQ2_XXS/Q4_K_M | 16K tokens | 9.78 GiB | 8.43 GB |
| `ai/phi4:14B-Q4_0` | 15B | Q4_0 | 16K tokens | 9.16 GiB | 7.80 GB |
| `ai/phi4:14B-Q4_K_M` | 15B | IQ2_XXS/Q4_K_M | 16K tokens | 9.78 GiB | 8.43 GB |
| `ai/phi4:14B-F16` | 15B | F16 | 16K tokens | 27.97 GiB | 27.31 GB |

¹: VRAM estimated based on model characteristics.

> `latest` → `14B-Q4_K_M`

## Use this AI model with Docker Model Runner

First, pull the model:

```bash
docker model pull ai/phi4
```

Then run the model:

```bash
docker model run ai/phi4
```

For more information on Docker Model Runner, [explore the documentation](https://docs.docker.com/desktop/features/model-runner/).


## Considerations

- Phi-4 is optimized for single-turn queries rather than long multi-turn conversations.
- Hallucinations may occur, particularly in factual knowledge recall.
- Instruction-following capabilities are less robust than some larger models.

## Benchmark performance

| Category                     | Benchmark  | phi-4 | phi-3  |
|------------------------------|------------|-------|--------|
| Popular Aggregated Benchmark | MMLU       | 84.8  | 77.9   |
| Science                      | GPQA       | 56.1  | 31.2   |
| Math                         | MGSM       | 80.6  | 53.5   |
|                              | MATH       | 80.4  | 44.6   |
| Code Generation              | HumanEval  | 82.6  | 67.8   |
| Factual Knowledge            | SimpleQA   | 3.0   | 7.6    |
| Reasoning                    | DROP       | 75.5  | 68.3   |

## Links

- [Introducing Phi-4](https://techcommunity.microsoft.com/blog/aiplatformblog/introducing-phi-4-microsoft%E2%80%99s-newest-small-language-model-specializing-in-comple/4357090)
- [Phi-4 Technical Report (arXiv)](https://arxiv.org/abs/2412.08905)
