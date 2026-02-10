# Qwen3 VL
*GGUF version by Unsloth*

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/qwen-280x184-overview@2x.svg)

Meet Qwen3-VL — the most powerful vision-language model in the Qwen series to date.

This generation delivers comprehensive upgrades across the board: superior text understanding & generation, deeper visual perception & reasoning, extended context length, enhanced spatial and video dynamics comprehension, and stronger agent interaction capabilities.

Key Enhancements:
- **Visual Agent:** Operates PC/mobile GUIs—recognizes elements, understands functions, invokes tools, completes tasks. 
- **Visual Coding Boost:** Generates Draw.io/HTML/CSS/JS from images/videos.
- **Advanced Spatial Perception:** Judges object positions, viewpoints, and occlusions; provides stronger 2D grounding and enables 3D grounding for spatial reasoning and embodied AI.
- **Long Context & Video Understanding:** Native 256K context, expandable to 1M; handles books and hours-long video with full recall and second-level indexing.
- **Enhanced Multimodal Reasoning:** Excels in STEM/Math—causal analysis and logical, evidence-based answers.
- **Upgraded Visual Recognition:** Broader, higher-quality pretraining is able to “recognize everything”—celebrities, anime, products, landmarks, flora/fauna, etc.
- **Expanded OCR: Supports 32 languages (up from 19):** robust in low light, blur, and tilt; better with rare/ancient characters and jargon; improved long-document structure parsing.
- **Text Understanding on par with pure LLMs:** Seamless text–vision fusion for lossless, unified comprehension.
---

## Model Architecture Updates:

![arc](https://github.com/docker/model-cards/raw/refs/heads/main/images/qwen3vl_arc.jpg)
1. **Interleaved-MRoPE:** Full‑frequency allocation over time, width, and height via robust positional embeddings, enhancing long‑horizon video reasoning.
2. **DeepStack:** Fuses multi‑level ViT features to capture fine‑grained details and sharpen image–text alignment.
3. **Text–Timestamp Alignment:** Moves beyond T‑RoPE to precise, timestamp‑grounded event localization for stronger video temporal modeling.

This is the weight repository for Qwen3-VL-8B-Instruct.

---

## Available model variants

| Model variant | Parameters | Quantization | Context window | VRAM¹ | Size |
|---------------|------------|--------------|----------------|------|-------|
| `ai/qwen3-vl:8B`<br><br>`ai/qwen3-vl:8B-UD-Q4_K_XL`<br><br>`ai/qwen3-vl:latest` | 8B | MOSTLY_Q4_K_M | 262K tokens | 5.91 GiB | 4.79 GB |
| `ai/qwen3-vl:2B-BF16` | 2B | MOSTLY_BF16 | 262K tokens | 4.38 GiB | 3.21 GB |
| `ai/qwen3-vl:2B-Q8_K_XL` | 2B | MOSTLY_Q8_0 | 262K tokens | 3.34 GiB | 2.17 GB |
| `ai/qwen3-vl:2B-UD-Q4_K_XL` | 2B | MOSTLY_Q4_K_M | 262K tokens | 2.22 GiB | 1.05 GB |
| `ai/qwen3-vl:4B-Q8_K_XL` | 4B | MOSTLY_Q8_0 | 262K tokens | 6.13 GiB | 4.70 GB |
| `ai/qwen3-vl:8B-Q8_K_XL` | 8B | MOSTLY_Q8_0 | 262K tokens | 10.36 GiB | 10.08 GB |
| `ai/qwen3-vl:32B-Q8_K_XL` | 32B | MOSTLY_Q8_0 | 262K tokens | 37.46 GiB | 36.76 GB |
| `ai/qwen3-vl:32B-UD-Q4_K_XL` | 32B | MOSTLY_Q4_K_M | 262K tokens | 20.41 GiB | 18.67 GB |
| `ai/qwen3-vl:4B-BF16` | 4B | MOSTLY_BF16 | 262K tokens | 8.92 GiB | 7.49 GB |
| `ai/qwen3-vl:4B-UD-Q4_K_XL` | 4B | MOSTLY_Q4_K_M | 262K tokens | 3.80 GiB | 2.37 GB |
| `ai/qwen3-vl:8B-BF16` | 8B | MOSTLY_BF16 | 262K tokens | 15.54 GiB | 15.26 GB |

¹: VRAM estimated based on model characteristics.

> `latest` → `8B`

## 🐳 Using this model with Docker Model Runner

Run the model:

```bash
docker model run ai/qwen3-vl
```

For more information, check out the [Docker Model Runner docs](https://docs.docker.com/desktop/features/model-runner/).

---

## 🔗 Links

- [Qwen3-VL](https://github.com/QwenLM/Qwen3-VL)
- [Unsloth Dynamic 2.0 GGUF](https://docs.unsloth.ai/basics/unsloth-dynamic-2.0-ggufs)