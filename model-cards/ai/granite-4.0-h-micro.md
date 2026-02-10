# Granite 4.0 H Micro

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/ibm-280x184-overview.svg)

## Description
Granite-4.0-H-Micro is a 3B parameter long-context instruct model finetuned from Granite-4.0-H-Micro-Base using a combination of open source instruction datasets with permissive license and internally collected synthetic datasets. This model is developed using a diverse set of techniques with a structured chat format, including supervised finetuning, model alignment using reinforcement learning, and model merging. Granite 4.0 instruct models feature improved instruction following (IF) and tool-calling capabilities, making them more effective in enterprise applications.

## Characteristics

| Attribute             | Details                                                                                                                           |
|-----------------------|-----------------------------------------------------------------------------------------------------------------------------------|
| **Provider**          | Granite Team, IBM                                                                                                                 |
| **Architecture**      | granitehybrid                                                                                                                     |
| **Cutoff date**       | Not disclosed                                                                                                                     |
| **Languages**         | English, German, Spanish, French, Japanese, Portuguese, Arabic, Czech, Italian, Korean, Dutch, Chinese (extensible via finetuning) |
| **Tool calling**      | ✅                                                                                                                                 |
| **Input modalities**  | Text                                                                                                                              |
| **Output modalities** | Text                                                                                                                              |
| **License**           | Apache 2.0                                                                                                                        |

## Available model variants

| Model variant | Parameters | Quantization | Context window | VRAM¹ | Size |
|---------------|------------|--------------|----------------|------|-------|
| `ai/granite-4.0-h-micro:3B`<br><br>`ai/granite-4.0-h-micro:3B-Q4_K_M`<br><br>`ai/granite-4.0-h-micro:latest` | 3.2B | MOSTLY_Q4_K_M | 1M tokens | 2.32 GiB | 1.81 GB |

¹: VRAM estimated based on model characteristics.

> `latest` → `3B`

## Use this AI model with Docker Model Runner

```bash
docker model run ai/granite-4.0-h-micro
```

## Considerations

- Optimized for instruction following, tool/function calling, and long-context (up to 128K tokens) scenarios.
- Strong generalist capabilities: summarization, classification, extraction, QA/RAG, coding, function-calling, and multilingual dialogue.
- Multilingual: best performance in English; a few-shot approach or light finetuning can help close gaps for other languages.
- Safety & reliability: despite alignment, the model can still produce inaccurate or biased outputs—apply domain-specific evaluation and guardrails.
- Infrastructure note: trained on NVIDIA GB200 NVL72 at CoreWeave; use acceleration libraries (e.g., accelerate, optimized attention/KV cache settings) for efficient inference.

## Benchmark performance

| Category               | Metric                      | Granite-4.0-h-Micro |
|------------------------|-----------------------------|---------------------|
| **General Tasks**      |                             |                     |
|                        | MMLU (5-shot)               | 67.43               |
|                        | MMLU-Pro (5-shot, CoT)      | 43.48               |
|                        | BBH (3-shot, CoT)           | 69.36               |
|                        | AGI EVAL (0-shot, CoT)      | 59.00               |
|                        | GPQA (0-shot, CoT)          | 32.15               |
| **Alignment Tasks**    |                             |                     |
|                        | AlpacaEval 2.0              | 31.49               |
|                        | IFEval (Instruct, Strict)   | 86.94               |
|                        | IFEval (Prompt, Strict)     | 81.71               |
|                        | IFEval (Average)            | 84.32               |
|                        | ArenaHard                   | 36.15               |
| **Math Tasks**         |                             |                     |
|                        | GSM8K (8-shot)              | 81.35               |
|                        | GSM8K Symbolic (8-shot)     | 77.50               |
|                        | Minerva Math (0-shot, CoT)  | 66.44               |
|                        | DeepMind Math (0-shot, CoT) | 43.83               |
| **Code Tasks**         |                             |                     |
|                        | HumanEval (pass@1)          | 81.00               |
|                        | HumanEval+ (pass@1)         | 75.00               |
|                        | MBPP (pass@1)               | 73.00               |
|                        | MBPP+ (pass@1)              | 64.00               |
|                        | CRUXEval-O (pass@1)         | 41.25               |
|                        | BigCodeBench (pass@1)       | 37.90               |
| **Tool Calling Tasks** |                             |                     |
|                        | BFCL v3                     | 57.56               |
| **Multilingual Tasks** |                             |                     |
|                        | MULTIPLE (pass@1)           | 49.46               |
|                        | MMMLU (5-shot)              | 55.19               |
|                        | INCLUDE (5-shot)            | 50.51               |
|                        | MGSM (8-shot)               | 44.48               |
| **Safety**             |                             |                     |
|                        | SALAD-Bench                 | 96.28               |
|                        | AttaQ                       | 84.44               |


## Links
- https://www.ibm.com/granite
- https://www.ibm.com/granite/docs/
- https://ibm.biz/granite-learning-resources
