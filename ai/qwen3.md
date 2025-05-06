# Qwen3

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/qwen-280x184-overview@2x.svg)

Qwen3 is the latest generation in the Qwen LLM family, designed for top-tier performance in coding, math, reasoning, and language tasks. It includes both dense and Mixture-of-Experts (MoE) models, offering flexible deployment from lightweight apps to large-scale research.

Qwen3 introduces dual reasoning modes—"thinking" for complex tasks and "non-thinking" for fast responses—giving users dynamic control over performance. It outperforms prior models in reasoning, instruction following, and code generation, while excelling in creative writing and dialogue.

With strong agentic and tool-use capabilities and support for over 100 languages, Qwen3 is optimized for multilingual, multi-domain applications.

---

## 📌 Characteristics

| Attribute             | Value             |
|-----------------------|-------------------|
| **Provider**          | Alibaba Cloud     |
| **Architecture**      | qwen3             |
| **Cutoff date**       | April 2025 (est.) |
| **Languages**         | 119 languages from multiple families  (Indo European, Sino-Tibetan, Afro-Asiatic, Austronesian, Dravidian, Turkic, Tai-Kadai, Uralic, Astroasiatic) including others like Japanese, Basque, Haitian,... |
| **Tool calling**      | ✅                |
| **Input modalities**  | Text              |
| **Output modalities** | Text              |
| **License**           | Apache 2.0        |

---


## Available model variants

| Model variant | Parameters | Quantization | Context window | VRAM¹ | Size |
|---------------|------------|--------------|----------------|------|-------|
| `ai/qwen3:latest`<br><br>`ai/qwen3:8B-Q4_K_M` | 8B | IQ2_XXS/Q4_K_M | 41K tokens | 5.31 GB | 4.68 GB |
| `ai/qwen3:0.6B-Q4_0` | 0.6B | Q4_0 | 41K tokens | 0.83 GB | 441.67 MB |
| `ai/qwen3:0.6B-Q4_K_M` | 0.6B | IQ2_XXS/Q4_K_M | 41K tokens | 0.62 GB | 456.11 MB |
| `ai/qwen3:0.6B-F16` | 0.6B | F16 | 41K tokens | 1.79 GB | 1.40 GB |
| `ai/qwen3:30B-A3B-F16` | 30B-A3B | F16 | 41K tokens | 680.86 GB | 56.89 GB |
| `ai/qwen3:30B-A3B-Q4_K_M` | 30B-A3B | IQ2_XXS/Q4_K_M | 41K tokens | 90.90 GB | 17.28 GB |
| `ai/qwen3:8B-Q4_0` | 8B | Q4_0 | 41K tokens | 8.03 GB | 4.44 GB |
| `ai/qwen3:8B-Q4_K_M` | 8B | IQ2_XXS/Q4_K_M | 41K tokens | 5.31 GB | 4.68 GB |
| `ai/qwen3:8B-F16` | 8B | F16 | 41K tokens | 20.88 GB | 15.26 GB |

¹: VRAM estimated based on model characteristics.

> `latest` → `8B-Q4_K_M`

## 🧠 Intended uses

Qwen3-8B is designed for a wide range of advanced natural language processing tasks:

- Supports both **Dense and Mixture-of-Experts (MoE)** model architectures, available in sizes including 0.6B, 1.7B, 4B, 8B, 14B, 32B, and large MoE variants like 30B-A3B and 235B-A22B.
- Enables **seamless switching between thinking and non-thinking modes**:
  - *Thinking mode*: optimized for complex logical reasoning, math, and code generation.
  - *Non-thinking mode*: tuned for efficient, general-purpose dialogue and chat.
- Offers **significant improvements in reasoning performance**, outperforming previous QwQ (in thinking mode) and Qwen2.5-Instruct (in non-thinking mode) models on mathematics, code generation, and commonsense reasoning benchmarks.
- Delivers **superior human alignment** and excels at: Creative writing, Role-playing, Multi-turn dialogue, Instruction following with immersive conversations.
- Provides strong **agent capabilities**, including: Integration with external tools and best-in-class performance in complex agent-based workflows across both thinking and unthinking modes.
- Offers support for **100+ languages and dialects**, with robust multilingual instruction following and translation abilities.

---

## Considerations

- **Thinking Mode Switching**  
  Qwen3 supports a soft switch mechanism via `/think` and `/no_think` prompts (when `enable_thinking=True`). This allows dynamic control over the model's reasoning depth during multi-turn conversations.
- **Tool Calling with Qwen-Agent**  
  For agentic tasks, use **Qwen-Agent**, which simplifies integration of external tools through built-in templates and parsers, minimizing the need for manual tool-call handling.
> **Note:** Qwen3 models use a new naming convention: post-trained models no longer include the `-Instruct` suffix (e.g., `Qwen3-32B` replaces `Qwen2.5-32B-Instruct`), and base models now end with `-Base`.

---

## 🐳 Using this model with Docker Model Runner

First, pull the model:

```bash
docker model pull ai/qwen3
```

Then run the model:

```bash
docker model run ai/qwen3
```

For more information, check out the [Docker Model Runner docs](https://docs.docker.com/desktop/features/model-runner/).

---

## Benchmarks

| Category                    | Benchmark  | Qwen3 |
|-----------------------------|------------|-------|
| General Tasks               | MMLU       | 87.81 |
|                             | MMLU-Redux | 87.40 |
|                             | MMLU-Pro   | 68.18 |
|                             | SuperGPQA  | 44.06 |
|                             | BBH        | 88.87 |
| Mathematics & Science Tasks | GPQA       | 47.47 |
|                             | GSM8K      | 94.39 |
|                             | MATH       | 71.84 |
| Multilingual Tasks          | MGSM       | 83.53 |
|                             | MMMLU      | 86.70 |
|                             | INCLUDE    | 73.46 |
| Code Tasks                  | EvalPlus   | 77.60 |
|                             | MultiPL-E  | 65.94 |
|                             | MBPP       | 81.40 |
|                             | CRUX-O     | 79.00 |

---

## 🔗 Links

- [Qwen3: Think Deeper, Act Faster](https://qwenlm.github.io/blog/qwen3/)