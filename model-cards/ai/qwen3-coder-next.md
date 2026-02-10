# Qwen3-Coder-Next
*GGUF version*

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/qwen-280x184-overview@2x.svg)

Open-weight language model specifically designed for coding agents and local development with exceptional efficiency.

## Intended uses

Highly efficient coding assistant optimized for agent deployment:

- **Agentic coding excellence**: Long-horizon reasoning for complex coding tasks with robust tool-calling capabilities.
- **Complex tool usage**: Advanced function calling with recovery from execution failures.
- **IDE/CLI integration**: Seamless integration with multiple platforms (Claude Code, Qwen Code, Qoder, Kilo, Trae, Cline).
- **Large context comprehension**: Native support for 256K token context (262,144 tokens).

## Characteristics

| Attribute             | Details        |
|----------------------|----------------|
| **Provider**          | Qwen / Alibaba |
| **Architecture**      | MoE (Mixture of Experts, 80B total with 3B activated, 512 experts, 10 active, 1 shared) |
| **Cutoff date**       | January 2025 |
| **Languages**         | Multilingual; programming and natural languages |
| **Tool calling**      | Yes |
| **Input modalities**  | Text (code + natural language) |
| **Output modalities** | Text (code + natural language) |
| **License**           | Apache-2.0 |
| **Context length**    | 262,144 tokens (256K) |

## Technical Specifications

| Specification | Details |
|---|---|
| **Type** | Causal Language Model |
| **Total Parameters** | 80B (79B non-embedding) |
| **Activated Parameters** | 3B |
| **Hidden Dimension** | 2048 |
| **Layers** | 48 (Hybrid: 12 × (3 × (Gated DeltaNet → MoE) + 1 × (Gated Attention → MoE))) |
| **Attention Heads** | 16 Q heads, 2 KV heads, 256 head dimension |
| **Linear Attention** | 32 V heads, 16 QK heads, 128 head dimension |
| **Rotary Position Embedding** | 64 dimensions |

## Available model variants (GGUF)

| Quantization | Size |
|---|---|
| Q4_K_M (4-bit) | 48.4 GB |
| Q5_0 (5-bit) | 55 GB |
| Q5_K_M (5-bit) | 56.7 GB |
| Q6_K (6-bit) | 65.5 GB |
| Q8_0 (8-bit) | 84.8 GB |
| F16 (16-bit) | 159 GB |

## Performance & Efficiency

- Achieves performance comparable to models with **10–20x more active parameters**
- Highly cost-effective for agent deployment
- 3B activated parameters out of 80B total parameters

## Recommended Sampling Parameters

- **Temperature**: 1.0
- **Top-p**: 0.95
- **Top-k**: 40

## Long Context Support

- Natively supports up to 262,144 tokens
- Validated performance up to 131,072 tokens using **YaRN** scaling method

## Important Notes

- **Non-thinking mode only**: Does not generate `<think></think>` blocks
- `enable_thinking=False` parameter is no longer required
- Adaptable to various scaffold templates

## Links

- [Hugging Face (GGUF)](https://huggingface.co/Qwen/Qwen3-Coder-Next-GGUF)
- [Hugging Face (Original)](https://huggingface.co/Qwen/Qwen3-Coder-Next)
- [Blog](https://qwen.ai/blog?id=qwen3-coder-next)
- [GitHub](https://github.com/QwenLM/Qwen3-Coder)
- [Documentation](https://qwen.readthedocs.io/en/latest/)
- [llama.cpp Guide](https://qwen.readthedocs.io/en/latest/run_locally/llama.cpp.html)
