# Phi-4 Model Card

<img src="https://github.com/jalonsogo/model-cards/blob/4c39899ef2d3eff3bfe28253b557283c8933c811/logos/microsoft.svg" width="120" />

Phi-4 is a **14-billion parameter** language model developed by Microsoft Research. It is part of the Phi model family, emphasizing **data quality and reasoning capabilities** over pure scale.

## Characteristics

| Attribute             | Details       |
|---------------------- |--------------|
| **Provider**          | Microsoft     |
| **Architecture**      | phi3          |
| **Cutoff Date**       | June 2024 |
| **Languages**         | English (primary), German, Spanish, French, Portuguese, Italian, Hindi, Japanese |
| **Input Modalities**  | Text          |
| **Output Modalities** | Text          |
| **License**           | MIT           |

## Available Model Variants

| Model Variant        | Parameters | Quantization | Context Window | VRAM     | Size   | Download |
|----------------------|----------- |--------------|--------------- |--------- |------- |--------- |
| `ai/phi4:latest`     | 14B        | Q4_K_M           | 16K tokens     |  8.2GB¹  | 9.05GB | Link     |
| `ai/phi4:14B-F16`    | 14B        | F16          | 16K tokens     |  21.5GB¹ | 29.3GB | Link     |
| `ai/phi4:14B-Q4_K_M` | 14B        | Q4_K_M           | 16K tokens     |  8.2GB¹  | 9.05GB | Link     |
¹: VRAM estimates based on model characteristics.

## Intended Uses

Phi-4 is designed for:
- **STEM reasoning & problem-solving** (math, physics, coding, logic)
- **Memory and compute-efficient applications**
- **Educational and research applications**
- **Conversational AI and chatbot experiences**

## Considerations

- Phi-4 is optimized for **single-turn queries** rather than long multi-turn conversations.
- **Hallucinations** may occur, particularly in factual knowledge recall.
- **Instruction-following capabilities** are less robust than some larger models.

## How to Run This AI Model

You can pull the model using:
```
docker model pull ai/phi4
```

To run the model:
```
docker model run ai/phi4
```

## Benchmark Performance

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
