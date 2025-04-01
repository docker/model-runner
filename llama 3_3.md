# Llama 3.3

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/meta-280x184-overview@2x.svg)

Meta Llama 3.3 is a powerful 70B parameter multilingual language model designed by Meta for text-based tasks like chat and content generation. The instruction-tuned version is optimized for multilingual dialogue and performs better than many open-source and commercial models on common benchmarks.

## Characteristics

| Attribute             | Details        |
|---------------------- |----------------|
| **Provider**          | Meta           |
| **Architecture**      | llama          |
| **Cutoff Date**       | December 2023  |
| **Languages**         | English, German, French, Italian, Portuguese, Hindi, Spanish, and Thai.|
| **Tool Calling**      | ✅             |
| **Input Modalities**  | Text           |
| **Output Modalities** | Text and Code  |
| **License**           | [Llama 3.3 Community license](https://github.com/meta-llama/llama-models/blob/main/models/llama3_3/LICENSE)     |

## Available Model Variants

| Model Variant                                | Parameters | Quantization   | Context Window | VRAM      | Size   | 
|--------------------------------------------- |----------- |--------------- |--------------- |---------- |------- |
| `ai/llama3.3latest` `ai/llama3.3:70B-Q4_K_M` | 70B        | Q4_K_M         | 128K           | 42GB¹     | 42.5GB | 

¹: VRAM estimates based on model characteristics.

## Intended Uses

- **Multilingual Assistant-like Chat**: Using instruction-tuned models for conversational AI across multiple languages, enabling natural and context-aware interactions in various linguistic settings.

- **Coding Support and Software Development Tasks**: Leveraging language models to assist with code generation, debugging, documentation, and other software engineering workflows.

- **Multilingual Content Creation and Localization**: Generating and adapting written content across different languages and cultures, supporting global communication and engagement.

- **Knowledge-based Applications**: Integrating LLMs with structured or unstructured data sources to answer questions, extract insights, or support decision-making.

- **General Natural Language Generation**: Various NLG tasks such as summarization, translation, or content generation across different domains.

- **Synthetic Data Generation (Synth)**: Creating realistic, high-quality synthetic text data to augment datasets for training, testing, or anonymization purposes.

## How to Run This AI Model

You can pull the model using

```
docker model pull ai/llama3.3:latest
```

To run the model:

```
docker model pull ai/llama3.3:latest
```


## Benchmark Performance

| Category     | Benchmark                | Llama-3.3 70B Instruct |
|--------------|--------------------------|------------------------|
| General      | MMLU (CoT)               | 86.0                   |
|              | MMLU Pro (CoT)           | 68.9                   |
| Steerability | IFEval                   | 92.1                   |
| Reasoning    | GPQA Diamond (CoT)       | 50.5                   |
| Code         | HumanEval                | 88.4                   |
|              | MBPP EvalPlus (base)     | 87.6                   |
| Math         | MATH (CoT)               | 77.0                   |
| Tool Use     | BFCL v2                  | 77.3                   |
| Multilingual | MGSM                     | 91.1                   |


## Links
- [Introducing Meta Llama 3: The most capable openly available LLM to date](https://ai.meta.com/blog/meta-llama-3/)
- [The Llama 3 Herd of Models](https://arxiv.org/pdf/2407.21783)
