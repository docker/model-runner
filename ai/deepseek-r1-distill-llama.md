# Deepseek-R1-Distill-Llama

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/deepseek-280x184-overview@2x.svg)

DeepSeek introduced its first-generation reasoning models, DeepSeek-R1-Zero and DeepSeek-R1, leveraging reinforcement learning to enhance reasoning performance, with DeepSeek-R1 achieving state-of-the-art results and open-sourcing multiple distilled models.

The models provided here are the distill-llama variants, which are llama based models that have been fine-tuned on the responses and reasoning output of the full DeepSeek-R1 model.

## Intended uses

Deepseek-R1-Distill-Llama can help with:
- **Software development:** Generates code, debugs, and explains complex concepts.
- **Mathematics:** Solves and explains complex problems for research and education.
- **Content creation and editing:** Writes, edits, and summarizes content for various industries.
- **Customer service:** Powers chatbots to engage users and answer queries.
- **Data analysis:** Extracts insights and generates reports from large datasets.
- **Education:** Acts as a digital tutor, providing clear explanations and personalized lessons.

## Characteristics

| Attribute             | Details          |
|---------------------- |----------------- |
| **Provider**          | Deepseek         |
| **Architecture**      | llama            |
| **Cutoff date**       | May 2024ⁱ        |
| **Languages**         | English, Chinese |
| **Tool calling**      | ✅               |
| **Input modalities**  | Text             |
| **Output modalities** | Text             |
| **License**           | [MIT](https://github.com/deepseek-ai/DeepSeek-R1/blob/main/LICENSE)           |

i: Estimated

## Available model variants

| Model variant | Parameters | Quantization | Context window | VRAM¹ | Size |
|---------------|------------|--------------|----------------|------|-------|
| `ai/deepseek-r1-distill-llama:latest`<br><br>`ai/deepseek-r1-distill-llama:8B-Q4_K_M` | 8B | IQ2_XXS/Q4_K_M | 131K tokens | 2.31 GB | 4.58 GB |
| `ai/deepseek-r1-distill-llama:8B-Q4_0` | 8B | Q4_0 | 131K tokens | 5.03 GB | 4.33 GB |
| `ai/deepseek-r1-distill-llama:8B-F16` | 8B | F16 | 131K tokens | 17.88 GB | 14.96 GB |
| `ai/deepseek-r1-distill-llama:70B-Q4_0` | 70B | Q4_0 | 131K tokens | 44.00 GB | 37.22 GB |
| `ai/deepseek-r1-distill-llama:70B-Q4_K_M` | 70B | IQ2_XXS/Q4_K_M | 131K tokens | 20.17 GB | 39.59 GB |

¹: VRAM estimated based on model characteristics.

> `latest` → `8B-Q4_K_M`

## Use this AI model with Docker Model Runner

First, pull the model:

```bash
docker model pull ai/deepseek-r1-distill-llama
```

Then run the model:

```bash 
docker model run ai/deepseek-r1-distill-llama
```

For more information on Docker Model Runner, [explore the documentation](https://docs.docker.com/desktop/features/model-runner/).

## Usage tips

- Set the temperature between 0.5 and 0.7 (recommended: 0.6) to avoid repetition or incoherence.
- Do not use a system prompt. Include all instructions within the user prompt.
- For math problems, add a directive like: "Please reason step by step and enclose the final answer in \boxed{}."

This model is sensitive to prompts. Few-shot prompting consistently degrades its performance. Therefore, we
recommend you directly describe the problem and specify the output format using a
zero-shot setting for optimal results.

## Benchmark performance

| Category    | Benchmark                   | DeepSeek R1  |
|-------------|-----------------------------|------------- |
| **English** |                             |              |
|             | MMLU (Pass@1)               | 90.8         |
|             | MMLU-Redux (EM)             | 92.9         |
|             | MMLU-Pro (EM) |             | 84.0         |
|             | DROP (3-shot F1) |          | 92.2         |
|             | IF-Eval (Prompt Strict) |   | 83.3         |
|             | GPQA-Diamond (Pass@1) |     | 71.5         |
|             | SimpleQA (Correct) |        | 30.1         |
|             | FRAMES (Acc.) |             | 82.5         |
|             | AlpacaEval2.0 (LC-winrate)  | 87.6         |
|             | ArenaHard (GPT-4-1106)      | 92.3         |
| **Code**    |                             |              |
|             | LiveCodeBench (Pass@1-COT)  | 65.9         |
|             | Codeforces (Percentile)     | 96.3         |
|             | Codeforces (Rating)         | 2029         |
|             | SWE Verified (Resolved)     | 49.2         |
|             | Aider-Polyglot (Acc.)       | 53.3         |
| **Math**    |                             |              |
|             | AIME 2024 (Pass@1)          | 79 .8        |
|             | MATH-500 (Pass@1)           | 97.3         |
|             | CNMO 2024 (Pass@1)          | 78.8         |
| **Chinese** |                             |              |
|             | CLUEWSC (EM)                | 92.8         |
|             | C-Eval (EM)                 | 91.8         |
|             | C-SimpleQA (Correct)        | 63.7         |


## Links
- [DeepSeek-R1: Incentivizing Reasoning Capability in LLMs via Reinforcement Learning](https://arxiv.org/abs/2501.12948)
