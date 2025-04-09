# QwQ

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/qwen-280x184-overview@2x.svg)

## Description
QwQ-32B is a 32-billion-parameter large language model designed to deliver high-level reasoning and intelligence. It achieves performance comparable to DeepSeek R1, a 671-billion-parameter model (with 37 billion activated), highlighting the efficiency of well-optimized foundation models trained on extensive world knowledge.
The model incorporates agent-like capabilities, allowing it to perform critical reasoning, utilize tools, and adapt its behavior based on real-time environmental feedback. These features enable QwQ-32B to handle complex tasks with deep thinking and dynamic decision-making.

## Characteristics

| Attribute             | Details            |
|---------------------- |--------------------|
| **Provider**          | Alibaba Cloud      |
| **Architecture**      | qwen2              |
| **Cutoff Date**       | -                  |
| **Languages**         | +29                |
| **Tool Calling**      | ❌                 |
| **Input Modalities**  | Text               |
| **Output Modalities** | Text               |
| **License**           | [Apache 2.0](https://github.com/QwenLM/QwQ/blob/main/LICENSE)|

## Available Model Variants

| Model Variant                              | Parameters | Quantization | Context Window | VRAM    | Size  | 
|--------------------------------------------|------------|--------------|----------------|---------|-------|
| `ai/qwq:32B-F16`                           | 32.5B      | FP16         | 40K tokens     | 77GB¹   | 65.5GB|
| `ai/qwq:latest`<br><br>`ai/qwq:32B-Q4_K_M` | 32.5B      | Q4_K_M       | 40K tokens     | 19GB¹   | 18.8GB|

> `:latest` → `32B-Q4_K_M`

¹: VRAM estimated based on model characteristics.

## Intended Uses

QwQ-32B is designed for tasks requiring advanced reasoning and problem-solving abilities.

- **Mathematical Problem Solving**: Excels in complex mathematical computations and proofs.
- **Code Generation and Debugging**: Assists in writing and troubleshooting code across various programming languages.
- **General Problem-Solving**: Provides insightful solutions to diverse challenges requiring logical reasoning.

## Considerations

- **Language Mixing and Code-Switching**: The model may unexpectedly switch languages, affecting response clarity. 
- **Recursive Reasoning Loops**: Potential for circular reasoning patterns leading to lengthy, inconclusive responses. Use Temperature=0.6, TopP=0.95, MinP=0 to avoid this and use TopK between 20 and 40 to filter out rare token occurrences while maintaining the diversity of the generated output.
- **Performance Limitations**: While excelling in math and coding, it may underperform in common sense reasoning and nuanced language understanding.

## How to Run This AI Model

You can pull the model using
```
docker model pull ai/qwq
```

Run this model using:
```
docker model run ai/qwq
```


## Links
- [QwQ-32B: Embracing the Power of Reinforcement Learning](https://qwenlm.github.io/blog/qwq-32b/)
