# Llama 3.3

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/meta-280x184-overview@2x.svg)

Meta Llama 3.3 is a powerful 70B parameter multilingual language model designed by Meta for text-based tasks like chat and content generation. The instruction-tuned version is optimized for multilingual dialogue and performs better than many open-source and commercial models on common benchmarks.

## Intended uses

- **Multilingual assistant-like chat**: Using instruction-tuned models for conversational AI across multiple languages, enabling natural and context-aware interactions in various linguistic settings.

- **Coding support and software development tasks**: Leveraging language models to assist with code generation, debugging, documentation, and other software engineering workflows.

- **Multilingual content creation and localization**: Generating and adapting written content across different languages and cultures, supporting global communication and engagement.

- **Knowledge-based applications**: Integrating LLMs with structured or unstructured data sources to answer questions, extract insights, or support decision-making.

- **General natural language generation**: Various NLG tasks such as summarization, translation, or content generation across different domains.

- **Synthetic data generation (synth)**: Creating realistic, high-quality synthetic text data to augment datasets for training, testing, or anonymization purposes.

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
| **License**           | [Llama 3.3 Community license](https://github.com/meta-llama/llama-models/blob/main/models/llama3_3/LICENSE)     |

## Available model variants

| Model variant                                        | Parameters | Quantization   | Context window | VRAM      | Size   | 
|----------------------------------------------------- |----------- |--------------- |--------------- |---------- |------- |
| `ai/llama3.3:latest`<br><br>`ai/llama3.3:70B-Q4_K_M` | 70B        | Q4_K_M         | 128K           | 42GB¹     | 42.5GB | 

¹: VRAM estimates based on model characteristics.

> `:latest` → `70B-Q4_K_M`

## Use this AI model with Docker Model Runner

First, pull the model:

```bash
docker model pull ai/llama3.3
```

Then run the model:

```bash
docker model run ai/llama3.3
```

For more information on Docker Model Runner, [explore the documentation](https://docs.docker.com/desktop/features/model-runner/).

## Benchmark performance

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
