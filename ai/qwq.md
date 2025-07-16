# QwQ

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/qwen-280x184-overview@2x.svg)

## Description
QwQ-32B is a 32-billion-parameter large language model designed to deliver high-level reasoning and intelligence. It achieves performance comparable to DeepSeek R1, a 671-billion-parameter model (with 37 billion activated), highlighting the efficiency of well-optimized foundation models trained on extensive world knowledge.
The model incorporates agent-like capabilities, allowing it to perform critical reasoning, utilize tools, and adapt its behavior based on real-time environmental feedback. These features enable QwQ-32B to handle complex tasks with deep thinking and dynamic decision-making.

## Intended uses

QwQ-32B is designed for tasks requiring advanced reasoning and problem-solving abilities.

- **Mathematical problem solving**: Excels in complex mathematical computations and proofs.
- **Code generation and debugging**: Assists in writing and troubleshooting code across various programming languages.
- **General problem-solving**: Provides insightful solutions to diverse challenges requiring logical reasoning.

## Characteristics

| Attribute             | Details            |
|---------------------- |--------------------|
| **Provider**          | Alibaba Cloud      |
| **Architecture**      | qwen2              |
| **Cutoff date**       | -                  |
| **Languages**         | +29                |
| **Tool calling**      | ✅                 |
| **Input modalities**  | Text               |
| **Output modalities** | Text               |
| **License**           | [Apache 2.0](https://github.com/QwenLM/QwQ/blob/main/LICENSE)|

## Available model variants

| Model variant | Parameters | Quantization | Context window | VRAM¹ | Size |
|---------------|------------|--------------|----------------|------|-------|
| `ai/qwq:latest`<br><br>`ai/qwq:32B-Q4_K_M` | 32B | IQ2_XXS/Q4_K_M | 41K tokens | 19.72 GiB | 18.48 GB |
| `ai/qwq:32B-Q4_0` | 32B | Q4_0 | 41K tokens | 18.60 GiB | 17.35 GB |
| `ai/qwq:32B-Q4_K_M` | 32B | IQ2_XXS/Q4_K_M | 41K tokens | 19.72 GiB | 18.48 GB |
| `ai/qwq:32B-F16` | 32B | F16 | 41K tokens | 61.23 GiB | 61.03 GB |

¹: VRAM estimated based on model characteristics.

> `latest` → `32B-Q4_K_M`

## Use this AI model with Docker Model Runner

First, pull the model:

```bash
docker model pull ai/qwq
```

Then run the model:

```bash
docker model run ai/qwq
```

For more information on Docker Model Runner, [explore the documentation](https://docs.docker.com/desktop/features/model-runner/).

## Considerations

- **Language mixing and code-switching**: The model may unexpectedly switch languages, affecting response clarity. 
- **Recursive reasoning loops**: Potential for circular reasoning patterns leading to lengthy, inconclusive responses. Use Temperature=0.6, TopP=0.95, MinP=0 to avoid this and use TopK between 20 and 40 to filter out rare token occurrences while maintaining the diversity of the generated output.
- **Performance limitations**: While excelling in math and coding, it may underperform in common sense reasoning and nuanced language understanding.

## Links

- [QwQ-32B: Embracing the Power of Reinforcement Learning](https://qwenlm.github.io/blog/qwq-32b/)
