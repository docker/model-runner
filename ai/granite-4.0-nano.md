# Granite 4.0 Nano

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/ibm-280x184-overview.svg)

## Description
Granite-4.0-350M is a lightweight instruct model finetuned from Granite-4.0-350M-Base using a combination of open source instruction datasets with permissive license and internally collected synthetic datasets. This model is developed using a diverse set of techniques including supervised finetuning, reinforcement learning, and model merging.

## Characteristics

| Attribute             | Details                                                                                                                            |
|-----------------------|------------------------------------------------------------------------------------------------------------------------------------|
| **Provider**          | Granite Team, IBM                                                                                                                  |
| **Architecture**      | granitehybrid                                                                                                                      |
| **Cutoff date**       | Not disclosed                                                                                                                      |
| **Languages**         | English, German, Spanish, French, Japanese, Portuguese, Arabic, Czech, Italian, Korean, Dutch, Chinese (extensible via finetuning) |
| **Tool calling**      | ✅                                                                                                                                  |
| **Input modalities**  | Text                                                                                                                               |
| **Output modalities** | Text                                                                                                                               |
| **License**           | Apache 2.0                                                                                                                         |

## Intended use
Intended use: Granite 4.0 Nano instruct models feature strong instruction following capabilities bringing advanced AI capabilities within reach for on-device deployments and research use cases. Additionally, their compact size makes them well-suited for fine-tuning on specialized domains without requiring massive compute resources.

## Available model variants

| Model variant                                                                                     | Parameters | Quantization | Context window | VRAM¹    | Size      |
|---------------------------------------------------------------------------------------------------|------------|--------------|----------------|----------|-----------|
| `ai/granite-4.0-nano:1B`<br><br>`ai/granite-4.0-nano:1B-BF16`<br><br>`ai/granite-4.0-nano:latest` | 1B         | MOSTLY_BF16  | 131K tokens    | 3.89 GiB | 3.04 GB   |
| `ai/granite-4.0-nano:350M-BF16`                                                                   | 350M       | MOSTLY_BF16  | 33K tokens     | 1.29 GiB | 672.22 MB |

¹: VRAM estimated based on model characteristics.

> `latest` → `1B`

## Use this AI model with Docker Model Runner

```bash
docker model run ai/granite-4.0-nano
```

## Considerations

- Optimized for instruction following, tool/function calling, and long-context (up to 128K tokens) scenarios.
- Strong generalist capabilities: summarization, classification, extraction, QA/RAG, coding, function-calling, and multilingual dialogue.
- Multilingual: best performance in English; a few-shot approach or light finetuning can help close gaps for other languages.
- Safety & reliability: despite alignment, the model can still produce inaccurate or biased outputs—apply domain-specific evaluation and guardrails.
- Infrastructure note: trained on NVIDIA GB200 NVL72 at CoreWeave; use acceleration libraries (e.g., accelerate, optimized attention/KV cache settings) for efficient inference.

## Benchmark performance

| Benchmarks             | Metric           | 350M Dense | H 350M Dense | 1B Dense | H 1B Dense |
|------------------------|------------------|------------|--------------|----------|------------|
| **General Tasks**      |                  |            |              |          |            |
| MMLU                   | 5-shot           | 35.01      | 36.21        | 59.39    | 59.74      |
| MMLU-Pro               | 5-shot, CoT      | 12.13      | 14.38        | 34.02    | 32.86      |
| BBH                    | 3-shot, CoT      | 33.07      | 33.28        | 60.37    | 59.68      |
| AGI EVAL               | 0-shot, CoT      | 26.22      | 29.61        | 49.22    | 52.44      |
| GPQA                   | 0-shot, CoT      | 24.11      | 26.12        | 29.91    | 29.69      |
| **Alignment Tasks**    |                  |            |              |          |            |
| IFEval                 | Instruct, Strict | 61.63      | 67.63        | 80.82    | 82.37      |
| IFEval                 | Prompt, Strict   | 49.17      | 55.64        | 73.94    | 74.68      |
| IFEval                 | Average          | 55.40      | 61.63        | 77.38    | 78.53      |
| **Math Tasks**         |                  |            |              |          |            |
| GSM8K                  | 8-shot           | 30.71      | 39.27        | 76.35    | 69.83      |
| GSM Symbolic           | 8-shot           | 26.76      | 33.70        | 72.30    | 65.72      |
| Minerva Math           | 0-shot, CoT      | 13.04      | 5.76         | 45.28    | 49.40      |
| DeepMind Math          | 0-shot, CoT      | 8.45       | 6.20         | 34.00    | 34.98      |
| **Code Tasks**         |                  |            |              |          |            |
| HumanEval              | pass@1           | 39.00      | 38.00        | 74.00    | 73.00      |
| HumanEval+             | pass@1           | 37.00      | 35.00        | 69.00    | 68.00      |
| MBPP                   | pass@1           | 48.00      | 49.00        | 65.00    | 69.00      |
| MBPP+                  | pass@1           | 38.00      | 44.00        | 57.00    | 60.00      |
| CRUXEval-O             | pass@1           | 23.75      | 25.50        | 33.13    | 36.00      |
| BigCodeBench           | pass@1           | 11.14      | 11.23        | 30.18    | 29.12      |
| **Tool Calling Tasks** |                  |            |              |          |            |
| BFCL v3                | —                | 39.32      | 43.32        | 54.82    | 50.21      |
| **Multilingual Tasks** |                  |            |              |          |            |
| MULTIPLE               | pass@1           | 15.99      | 14.31        | 32.24    | 36.11      |
| MMMLU                  | 5-shot           | 28.23      | 27.95        | 45.00    | 49.43      |
| INCLUDE                | 5-shot           | 27.74      | 27.09        | 42.12    | 43.35      |
| MGSM                   | 8-shot           | 14.72      | 16.16        | 37.84    | 27.52      |
| **Safety**             |                  |            |              |          |            |
| SALAD-Bench            | —                | 97.12      | 96.55        | 93.44    | 96.40      |
| AttaQ                  | —                | 82.53      | 81.76        | 85.26    | 82.85      |


## Links
- https://www.ibm.com/granite
- https://www.ibm.com/granite/docs/
- https://ibm.biz/granite-learning-resources
