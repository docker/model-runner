# Llama 3.1

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/meta-280x184-overview@2x.svg)

​Meta Llama 3.1 is a collection of multilingual large language models (LLMs) available in 8B, 70B and 405B parameter sizes. These models are designed for text-based tasks, including chat and content generation. The instruction-tuned versions available here are optimized for multilingual dialogue use cases and have demonstrated superior performance compared to many open-source and commercial chat models on common industry benchmarks. 

## Intended uses

- **Assistant-like chat:** Instruction-tuned text-only models are optimized for multilingual dialogue, making them ideal for developing conversational AI assistants. ​

- **Natural language generation tasks:** Pretrained models can be adapted for various text-based applications, such as content creation, summarization, and translation. ​

- **Synthetic data generation:** Utilize the outputs of Llama 3.1 to create synthetic datasets, which can aid in training and improving other models. ​

- **Model distillation:** Leverage Llama 3.1 to enhance smaller models by transferring knowledge, resulting in more efficient and specialized AI systems, or by using it as a base model to fine-tune based on the knowledge of other bigger models (see `deepseek-r1-distill-llama` as an example) ​

- **Research purposes:** Employ Llama 3.1 in academic and scientific research to explore advancements in natural language processing and artificial intelligence. 

## Characteristics

| Attribute             | Details        |
|---------------------- |----------------|
| **Provider**          | Meta           |
| **Architecture**      | llama          |
| **Cutoff date**       | December 2023  |
| **Languages**         | English, German, French, Italian, Portuguese, Hindi, Spanish, and Thai.|
| **Tool calling**      | ✅             |
| **Input modalities**  | Text           |
| **Output modalities** | Text and Code  |
| **License**           | [Llama 3.1 Community license](https://github.com/meta-llama/llama-models/blob/main/models/llama3_1/LICENSE)|

## Available model variants

| Model variant | Parameters | Quantization | Context window | VRAM¹ | Size |
|---------------|------------|--------------|----------------|------|-------|
| `ai/llama3.1:latest`<br><br>`ai/llama3.1:8B-Q4_K_M` | 8B | IQ2_XXS/Q4_K_M | 131K tokens | 5.33 GiB | 4.58 GB |
| `ai/llama3.1:8B-Q4_K_M` | 8B | IQ2_XXS/Q4_K_M | 131K tokens | 5.33 GiB | 4.58 GB |
| `ai/llama3.1:8B-F16` | 8B | F16 | 131K tokens | 15.01 GiB | 14.96 GB |

¹: VRAM estimated based on model characteristics.

> `latest` → `8B-Q4_K_M`

## Use this AI model with Docker Model Runner

First, pull the model:

```bash
docker model pull ai/llama3.1
```

Then run the model:

```bash
docker model run ai/llama3.1
```

For more information on Docker Model Runner, [explore the documentation](https://docs.docker.com/desktop/features/model-runner/).

## Benchmark performance

| Category    | Benchmark                    | Llama 3.1 8B |
|-------------|------------------------------|--------------|
| General     | MMLU                         | 69.4         |
|             | MMLU (CoT)                   | 73.0         |
|             | MMLU-Pro (CoT)               | 48.3         |
|             | IFEval                       | 80.4         |
| Reasoning   | ARC-C                        | 83.4         |
|             | GPQA                         | 30.4         |
| Code        | HumanEval                    | 72.6         |
|             | MBPP ++ base version         | 72.8         |
|             | Multipl-E HumanEval          | 50.8         |
|             | Multipl-E MBPP               | 52.4         |
| Math        | GSM-8K (CoT)                 | 84.5         |
|             | MATH (CoT)                   | 51.9         |
| Tool Use    | API-Bank                     | 82.6         |
|             | BFCL                         | 76.1         |
|             | Gorilla Benchmark API Bench  | 8.2          |
|             | Nexus (0-shot)               | 38.5         |
| Multilingual| Multilingual MGSM (CoT)      | 68.9         |
|             | MMLU (5-shot) - Portuguese   | 62.12        |
|             | MMLU (5-shot) - Spanish      | 62.45        |
|             | MMLU (5-shot) - Italian      | 61.63        |
|             | MMLU (5-shot) - German       | 60.59        |
|             | MMLU (5-shot) - French       | 62.34        |
|             | MMLU (5-shot) - Hindi        | 50.88        |
|             | MMLU (5-shot) - Thai         | 50.32        |

## Links
- [https://ai.meta.com/blog/meta-llama-3-1/](https://ai.meta.com/blog/meta-llama-3-1/)
- [The Llama 3 Herd of Models](https://arxiv.org/pdf/2407.21783)
