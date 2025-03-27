# SmolLM2

![](https://huggingface.co/datasets/HuggingFaceTB/images/resolve/main/banner_smol.png)

SmolLM2-360M is a compact language model with 360 million parameters, designed to run efficiently on-device while performing a wide range of language tasks. Trained on 4 trillion tokens from a diverse mix of datasets—including FineWeb-Edu, DCLM, The Stack, and newly curated filtered sources—it delivers strong performance in instruction following, knowledge, and reasoning. The instruct version was developed through supervised fine-tuning (SFT) on a blend of public and proprietary datasets, followed by Direct Preference Optimization (DPO) using UltraFeedback.



## Characteristics

| Attribute             | Details       |
|---------------------- |---------------|
| **Provider**          | Hugging Face  |
| **Architecture**      | Llama         |
| **Cutoff Date**       | June 2024     |
| **Languages**         | English       |
| **Tool Calling**      | ✅           |
| **Input Modalities**  | Text          |
| **Output Modalities** | Text          |
| **License**           | [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) |


## Available Model Variants
| Model Variant         | Parameters | Quantization | Context Window | VRAM    | Size   | Download |
|-----------------------|----------- |--------------|--------------- |---------|------- |--------- |
| `ai/smollm2:latest`   | 360M       | F16          | 8K tokens      | -       | -      | Link     |
| `ai/smollm2:360M-F16` | 360M       | F16          | 8K tokens      | -       | -      | Link     |
| `ai/smollm2:360M-Q4_K_M`         | 360M       | Q4_K_M       | 8K tokens      | 204 MB¹ | -      | Link     |

¹: VRAM estimation.

## Intended Uses

SmolLM2 is designed for:

- **Chat Assistants** 
- **Text-extraction**
- **Rewriting and summarization**

## How to Run This AI Model

You can pull the model using:
```
docker model pull ai/smollm2
```

To run the model:
```
docker model run ai/smollm2
```

## Benchmark Performance

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
| Math	                        | GSM8K (5-shot)              | 3.2  |


## Links
- [SmolLM2: When Smol Goes Big -- Data-Centric Training of a Small Language Model](https://arxiv.org/abs/2502.02737) 
