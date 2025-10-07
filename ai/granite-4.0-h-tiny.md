# Granite 4.0 H Tiny

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/ibm-280x184-overview.svg)

## Description
Granite-4.0-H-Tiny is a 7B parameter long-context instruct model finetuned from Granite-4.0-H-Tiny-Base using a combination of open source instruction datasets with permissive license and internally collected synthetic datasets. This model is developed using a diverse set of techniques with a structured chat format, including supervised finetuning, model alignment using reinforcement learning, and model merging. Granite 4.0 instruct models feature improved instruction following (IF) and tool-calling capabilities, making them more effective in enterprise applications.

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
| `ai/granite-4.0-h-tiny:7B`<br><br>`ai/granite-4.0-h-tiny:7B-Q4_K_M`<br><br>`ai/granite-4.0-h-tiny:latest` | 64x994M | MOSTLY_Q4_K_M | 1M tokens | 4.41 GiB | 3.94 GB |

¹: VRAM estimated based on model characteristics.

> `latest` → `7B`

## Use this AI model with Docker Model Runner

```bash
docker model run ai/granite-4.0-h-tiny
```

## Considerations

- Optimized for instruction following, tool/function calling, and long-context (up to 128K tokens) scenarios.
- Strong generalist capabilities: summarization, classification, extraction, QA/RAG, coding, function-calling, and multilingual dialogue.
- Multilingual: best performance in English; a few-shot approach or light finetuning can help close gaps for other languages.
- Safety & reliability: despite alignment, the model can still produce inaccurate or biased outputs—apply domain-specific evaluation and guardrails.
- Infrastructure note: trained on NVIDIA GB200 NVL72 at CoreWeave; use acceleration libraries (e.g., accelerate, optimized attention/KV cache settings) for efficient inference.

## Benchmark performance

| Category               | Metric                      | Granite-4.0-h-Tiny |
|------------------------|-----------------------------|--------------------|
| **General Tasks**      |                             |                    |
|                        | MMLU (5-shot)               | 68.65              |
|                        | MMLU-Pro (5-shot, CoT)      | 44.94              |
|                        | BBH (3-shot, CoT)           | 66.34              |
|                        | AGI EVAL (0-shot, CoT)      | 62.15              |
|                        | GPQA (0-shot, CoT)          | 32.59              |
| **Alignment Tasks**    |                             |                    |
|                        | AlpacaEval 2.0              | 30.61              |
|                        | IFEval (Instruct, Strict)   | 84.78              |
|                        | IFEval (Prompt, Strict)     | 78.10              |
|                        | IFEval (Average)            | 81.44              |
|                        | ArenaHard                   | 35.75              |
| **Math Tasks**         |                             |                    |
|                        | GSM8K (8-shot)              | 84.69              |
|                        | GSM8K Symbolic (8-shot)     | 81.10              |
|                        | Minerva Math (0-shot, CoT)  | 69.64              |
|                        | DeepMind Math (0-shot, CoT) | 49.92              |
| **Code Tasks**         |                             |                    |
|                        | HumanEval (pass@1)          | 83.00              |
|                        | HumanEval+ (pass@1)         | 76.00              |
|                        | MBPP (pass@1)               | 80.00              |
|                        | MBPP+ (pass@1)              | 69.00              |
|                        | CRUXEval-O (pass@1)         | 39.63              |
|                        | BigCodeBench (pass@1)       | 41.06              |
| **Tool Calling Tasks** |                             |                    |
|                        | BFCL v3                     | 57.65              |
| **Multilingual Tasks** |                             |                    |
|                        | MULTIPLE (pass@1)           | 55.83              |
|                        | MMMLU (5-shot)              | 61.87              |
|                        | INCLUDE (5-shot)            | 53.12              |
|                        | MGSM (8-shot)               | 45.36              |
| **Safety**             |                             |                    |
|                        | SALAD-Bench                 | 97.77              |
|                        | AttaQ                       | 86.61              |

## Links
- https://www.ibm.com/granite
- https://www.ibm.com/granite/docs/
- https://ibm.biz/granite-learning-resources
