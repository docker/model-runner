# Mistral Nemo Instruct 2407

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/mistral-280x184-overview@2x.svg)

Mistral-Nemo-Instruct-2407 is an instruct fine-tuned large language model developed by Mistral AI and NVIDIA, optimized for multilingual tasks and instruction-following capabilities.
Compared to Mistral 7B, it is much better at following precise instructions, reasoning, handling multi-turn conversations, and generating code.

## Characteristics

| Attribute             | Details                                                                                 |
|-----------------------|-----------------------------------------------------------------------------------------|
| **Provider**          | Mistral AI & NVIDIA                                                                     |
| **Architecture**      | llama                                                                                   |
| **Cutoff Date**       | July 2024                                                                               |
| **Languages**         | English, French, German, Spanish, Italian, Portuguese, Russian, Chinese, Japanese       |
| **Tool Calling**      | ✅                                                                                      |
| **Input Modalities**  | Text                                                                                    |
| **Output Modalities** | Text                                                                                    |
| **License**           | Apache 2.0                                                                              |

## Available Model Variants

| Model Variant                                         | Parameters | Quantization | Context Window | VRAM   | Size  |
|-------------------------------------------------------|------------|--------------|----------------|--------|-------|
| `ai/mistral-nemo:12B-F16`                             | 12B        | FP16         | 128k tokens    | 28GB¹ | 24 GB |
| `ai/mistral-nemo:latest` `ai/mistral-nemo:12B-Q4_K_M` | 12B        | Q4 K M       | 128k tokens    | 7GB¹  | 7.1 GB|

¹: VRAM estimated based on model characteristics.

`:latest` →  `mistral-nemo:12B-Q4_K_M` 

## Intended Uses

Mistral-Nemo-Instruct-2407 is designed for instruction-following tasks and multilingual applications, including:

- **Conversational AI**: Developing interactive, multilingual chatbots.
- **Knowledge Retrieval**: Answering questions across multiple languages.
- **Code Assistance**: Generating and understanding code snippets.

## How to Run This AI Model

You can pull the model using:

```
docker model pull ai/mistral-nemo:latest
```

To run the model:

```
docker model run ai/mistral-nemo:latest
```


## Benchmark Performance

| Benchmark                   | Score |
|-----------------------------|-------|
| HellaSwag (0-shot)          | 83.5% |
| Winogrande (0-shot)         | 76.8% |
| OpenBookQA (0-shot)         | 60.6% |
| CommonSenseQA (0-shot)      | 70.4% |
| TruthfulQA (0-shot)         | 50.3% |
| MMLU (5-shot)               | 68.0% |
| TriviaQA (5-shot)           | 73.8% |
| NaturalQuestions (5-shot)   | 31.2% |

## Links
- [Mistral Nemo](https://mistral.ai/news/mistral-nemo)
