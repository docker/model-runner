# DeepCoder-14B

![Agentica](logos/agentica-280x184-overview.png)


DeepCoder-14B is a powerful AI model built to write and understand code, especially in longer and more complex tasks.  
It's based on an open model from DeepSeek and trained using reinforcement learning to make it even smarter and more capable.  
Despite being open and only 14 billion parameters, it performs similarly to OpenAI's o3-mini, which is a more closed and proprietary model.

---

## Available Model Variants

| Model Variant                | Parameters | Quantization | Context Window | VRAM  | Size    |
|------------------------------|------------|--------------|----------------|--------|--------|
| `deepcoder-preview:14B-F16`    | 14.77B     | F16          | 131,072        | 24GB¹  | 29.5GB |
| `deepcoder-preview:14B:latest` <br><br> `deepcoder-preview:14B-Q4_K_M` | 14.77B     | Q4_K_M       | 131,072        | 8GB¹   | 9GB    |

¹: VRAM estimated based on GGUF model characteristics.

---

## Characteristics

| Attribute             | Details          |
|-----------------------|------------------|
| **Provider**          | Agentica         |
| **Architecture**      | Qwen2            |
| **Cutoff Date**       | February 2025¹   |
| **Languages**         | English          |
| **Tool Calling**      | No               |
| **Input Modalities**  | Text             |
| **Output Modalities** | Text             |
| **License**           | MIT              |

¹: Estimated

---

## Intended Uses

DeepCoder-14B is purpose-built for advanced code reasoning, programming task solving, and long-context inference.

- **Competitive Coding**: Excels at benchmarks like Codeforces and LiveCodeBench.
- **Code Generation & Repair**: Strong at structured, logic-heavy tasks using synthetic and real-world code datasets.
- **Research**: Ideal for experimenting with reinforcement learning for LLMs (via GRPO+) and context-length scaling.

---

## Considerations

- **Prompting**: Avoid system prompts; keep instructions in the user message.
- **Sampling**: Use `temperature=0.6`, `top_p=0.95`.
- **max_tokens**: Recommend at least 64K for full potential.
- **Truncation**: Scores may degrade at shorter context lengths.

---

## How to Run This AI Model

You can pull the model using:

```bash
docker model pull ai/deepcoder-preview
```

To run the model:

```bash
docker model run ai/deepcoder-preview
```

---

## Benchmark Performance

| Benchmark         | Metric             | DeepCoder-14B |
|-------------------|--------------------|---------------|
| LiveCodeBench v5  | Pass@1             | 60.6%         |
| Codeforces        | Elo Rating         | 1936          |
| Codeforces        | Percentile         | 95.3          |
| HumanEval+        | Accuracy           | 92.6%         |

---

## Links

- [📖 Training Blog](https://code.blog/deepcoder)