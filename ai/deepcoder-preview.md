# DeepCoder-14B

![Agentica](https://github.com/docker/model-cards/raw/refs/heads/main/logos/agentica-280x184-overview.png)


DeepCoder-14B is a powerful AI model built to write and understand code, especially in longer and more complex tasks.  
It's based on an open model from DeepSeek and trained using reinforcement learning to make it even smarter and more capable.  
Despite being open and only 14 billion parameters, it performs similarly to OpenAI's o3-mini, which is a more closed and proprietary model.

## Intended uses

DeepCoder-14B is purpose-built for advanced code reasoning, programming task solving, and long-context inference.

- **Competitive coding**: Excels at benchmarks like Codeforces and LiveCodeBench.
- **Code generation and repair**: Strong at structured, logic-heavy tasks using synthetic and real-world code datasets.
- **Research**: Ideal for experimenting with reinforcement learning for LLMs (via GRPO+) and context-length scaling.

## Characteristics

| Attribute             | Details          |
|-----------------------|------------------|
| **Provider**          | Agentica         |
| **Architecture**      | Qwen2            |
| **Cutoff date**       | February 2025¹   |
| **Languages**         | English          |
| **Tool calling**      | No               |
| **Input modalities**  | Text             |
| **Output modalities** | Text             |
| **License**           | MIT              |

¹: Estimated

## Available model variants

| Model variant | Parameters | Quantization | Context window | VRAM¹ | Size |
|---------------|------------|--------------|----------------|------|-------|
| `ai/deepcoder-preview:latest`<br><br>`ai/deepcoder-preview:14B-Q4_K_M` | 14B | IQ2_XXS/Q4_K_M | 131K tokens | 4.03 GB | 8.37 GB |
| `ai/deepcoder-preview:14B-Q4_K_M` | 14B | IQ2_XXS/Q4_K_M | 131K tokens | 4.03 GB | 8.37 GB |
| `ai/deepcoder-preview:14B-F16` | 14B | F16 | 131K tokens | 31.29 GB | 27.51 GB |

¹: VRAM estimated based on model characteristics.

> `latest` → `14B-Q4_K_M`

## Use this AI model with Docker Model Runner

First, pull the model:

```bash
docker model pull ai/deepcoder-preview
```

Then run the model:

```bash
docker model run ai/deepcoder-preview
```

For more information on Docker Model Runner, [explore the documentation](https://docs.docker.com/desktop/features/model-runner/).


## Usage tips

- **Prompting**: Avoid system prompts; keep instructions in the user message.
- **Sampling**: Use `temperature=0.6`, `top_p=0.95`.
- **Token limits**: Allocate at least 64K to leverage full potential capability.
- **Truncation**: Scores may degrade at shorter context lengths.


## Benchmark performance

| Benchmark         | Metric             | DeepCoder-14B |
|-------------------|--------------------|---------------|
| LiveCodeBench v5  | Pass@1             | 60.6%         |
| Codeforces        | Elo Rating         | 1936          |
| Codeforces        | Percentile         | 95.3          |
| HumanEval+        | Accuracy           | 92.6%         |


## Links

- [📖 Training blog](https://code.blog/deepcoder)