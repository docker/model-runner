# Granite 4.0 H Small

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/ibm-280x184-overview.svg)

## Description
Granite-4.0-H-Small is a 32B parameter long-context instruct model finetuned from Granite-4.0-H-Small-Base using a combination of open source instruction datasets with permissive license and internally collected synthetic datasets. This model is developed using a diverse set of techniques with a structured chat format, including supervised finetuning, model alignment using reinforcement learning, and model merging. Granite 4.0 instruct models feature improved instruction following (IF) and tool-calling capabilities, making them more effective in enterprise applications.

## Characteristics

| Attribute             | Details                                                                                                                            |
|-----------------------|------------------------------------------------------------------------------------------------------------------------------------|
| **Provider**          | Granite Team, IBM                                                                                                                  |
| **Architecture**      | granitehybrid                                                                                                                     |
| **Cutoff date**       | Not disclosed                                                                                                                      |
| **Languages**         | English, German, Spanish, French, Japanese, Portuguese, Arabic, Czech, Italian, Korean, Dutch, Chinese (extensible via finetuning) |
| **Tool calling**      | ✅                                                                                                                                  |
| **Input modalities**  | Text                                                                                                                               |
| **Output modalities** | Text                                                                                                                               |
| **License**           | Apache 2.0                                                                                                                         |

## Available model variants

| Model variant | Parameters | Quantization | Context window | VRAM¹ | Size |
|---------------|------------|--------------|----------------|------|-------|
| `ai/granite-4.0-h-small:32B`<br><br>`ai/granite-4.0-h-small:32B-Q4_K_M`<br><br>`ai/granite-4.0-h-small:latest` | 32.21 B | MOSTLY_Q4_K_M | 1M tokens | 18.80 GiB | 18.14 GB |

¹: VRAM estimated based on model characteristics.

> `latest` → `32B`

## Use this AI model with Docker Model Runner

```bash
docker model run ai/granite-4.0-h-small
```

## Considerations

- Optimized for instruction following, tool/function calling, and long-context (up to 128K tokens) scenarios.
- Strong generalist capabilities: summarization, classification, extraction, QA/RAG, coding, function-calling, and multilingual dialogue.
- Multilingual: best performance in English; a few-shot approach or light finetuning can help close gaps for other languages.
- Safety & reliability: despite alignment, the model can still produce inaccurate or biased outputs—apply domain-specific evaluation and guardrails.
- Infrastructure note: trained on NVIDIA GB200 NVL72 at CoreWeave; use acceleration libraries (e.g., accelerate, optimized attention/KV cache settings) for efficient inference.

## Benchmark performance

| Category               | Metric                      | Granite-4.0-h-Small |
|------------------------|-----------------------------|---------------------|
| **General Tasks**      |                             |                     |
|                        | MMLU (5-shot)               | 78.44               |
|                        | MMLU-Pro (5-shot, CoT)      | 55.47               |
|                        | BBH (3-shot, CoT)           | 81.62               |
|                        | AGI EVAL (0-shot, CoT)      | 70.63               |
|                        | GPQA (0-shot, CoT)          | 40.63               |
| **Alignment Tasks**    |                             |                     |
|                        | AlpacaEval 2.0              | 42.48               |
|                        | IFEval (Instruct, Strict)   | 89.87               |
|                        | IFEval (Prompt, Strict)     | 85.22               |
|                        | IFEval (Average)            | 87.55               |
|                        | ArenaHard                   | 46.48               |
| **Math Tasks**         |                             |                     |
|                        | GSM8K (8-shot)              | 87.27               |
|                        | GSM8K Symbolic (8-shot)     | 87.38               |
|                        | Minerva Math (0-shot, CoT)  | 74.00               |
|                        | DeepMind Math (0-shot, CoT) | 59.33               |
| **Code Tasks**         |                             |                     |
|                        | HumanEval (pass@1)          | 88.00               |
|                        | HumanEval+ (pass@1)         | 83.00               |
|                        | MBPP (pass@1)               | 84.00               |
|                        | MBPP+ (pass@1)              | 71.00               |
|                        | CRUXEval-O (pass@1)         | 50.25               |
|                        | BigCodeBench (pass@1)       | 46.23               |
| **Tool Calling Tasks** |                             |                     |
|                        | BFCL v3                     | 64.69               |
| **Multilingual Tasks** |                             |                     |
|                        | MULTIPLE (pass@1)           | 57.37               |
|                        | MMMLU (5-shot)              | 69.69               |
|                        | INCLUDE (5-shot)            | 63.97               |
|                        | MGSM (8-shot)               | 38.72               |
| **Safety**             |                             |                     |
|                        | SALAD-Bench                 | 97.30               |
|                        | AttaQ                       | 86.64               |


## Links
- https://www.ibm.com/granite
- https://www.ibm.com/granite/docs/
- https://ibm.biz/granite-learning-resources
