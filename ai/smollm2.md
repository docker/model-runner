# SmolLM2

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/hugginfface-280x184-overview@2x.svg)

SmolLM2-360M is a compact language model with 360 million parameters, designed to run efficiently on-device while performing a wide range of language tasks. Trained on 4 trillion tokens from a diverse mix of datasets—including FineWeb-Edu, DCLM, The Stack, and newly curated filtered sources—it delivers strong performance in instruction following, knowledge, and reasoning. The instruct version was developed through supervised fine-tuning (SFT) on a blend of public and proprietary datasets, followed by Direct Preference Optimization (DPO) using UltraFeedback.

## Intended uses

SmolLM2 is designed for:

- **Chat assistants** 
- **Text-extraction**
- **Rewriting and summarization**

## Characteristics

| Attribute             | Details       |
|---------------------- |---------------|
| **Provider**          | Hugging Face  |
| **Architecture**      | Llama2        |
| **Cutoff date**       | June 2024     |
| **Languages**         | English       |
| **Tool calling**      | ✅            |
| **Input modalities**  | Text          |
| **Output modalities** | Text          |
| **License**           | [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) |


## Available model variants

| Model variant | Parameters | Quantization | Context window | VRAM¹ | Size |
|---------------|------------|--------------|----------------|------|-------|
| `ai/smollm2:latest`<br><br>`ai/smollm2:360M-Q4_K_M` | 360M | IQ2_XXS/Q4_K_M | 8K tokens | 1.23 GB | 256.35 MB |
| `ai/smollm2:135M-F16` | 135M | F16 | 8K tokens | 0.93 GB | 256.63 MB |
| `ai/smollm2:135M-Q2_K` | 135M | Q2_K | 8K tokens | 0.68 GB | 82.41 MB |
| `ai/smollm2:135M-Q4_0` | 135M | Q4_0 | 8K tokens | 0.72 GB | 85.77 MB |
| `ai/smollm2:135M-Q4_K_M` | 135M | IQ2_XXS/Q4_K_M | 8K tokens | 0.67 GB | 98.87 MB |
| `ai/smollm2:360M-F16` | 360M | F16 | 8K tokens | 1.93 GB | 690.24 MB |
| `ai/smollm2:360M-Q4_0` | 360M | Q4_0 | 8K tokens | 1.35 GB | 216.80 MB |

¹: VRAM estimated based on model characteristics.

> `latest` → `360M-Q4_K_M`

## Use this AI model with Docker Model Runner

First, pull the model:

```bash
docker model pull ai/smollm2
```

Then run the model:

```bash
docker model run ai/smollm2
```

For more information on Docker Model Runner, [explore the documentation](https://docs.docker.com/desktop/features/model-runner/).

## Benchmark performance

| Category                     | Benchmark                   | Score |
|------------------------------|---------------------------- |-------|
| Reasoning                    | HellaSwag                   | 54.5  |
| Science                      | OpenBookQA                  | 37.4  |
|                              | ARC                         | 53.0  |
| Reasoning                    | PIQA                        | 71.7  |
|                              | CommonsenseQA               | 38.0  |
|                              | Winogrande                  | 52.5  |
| Popular Aggregated Benchmark | MMLU (cloze)                | 35.8  |
|                              | TriviaQA (held-out)         | 16.9  |
| Math	                       | GSM8K (5-shot)              | 3.2   |


## Links

- [SmolLM2: When Smol Goes Big -- Data-Centric Training of a Small Language Model](https://arxiv.org/abs/2502.02737) 
