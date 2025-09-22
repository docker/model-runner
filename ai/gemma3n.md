
# Gemma 3n

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/gemma-280x184-overview@2x.svg)

Gemma 3n is a compact, multimodal AI model from Google DeepMind, designed for efficiency on low-resource devices. It supports text, image, audio, and video input, with open weights and support for over 140 languages. With optimized parameter usage and strong safety features, it builds on the Gemma family to extend lightweight, high-performance foundation models.

## Intended uses

Gemma 3n models are designed for:

- **Text generation:** Compose poems, code, scripts, summaries, and more.  
- **Conversational AI:** Power virtual agents and customer service assistants.  
- **Multimodal data extraction:** Understand and summarize image, audio, and video content.  
- **Text summarization:** Generate concise overviews of articles, papers, or transcripts.  
- **Language learning:** Provide grammar suggestions and writing feedback.  
- **Educational research:** Assist with data analysis, question answering, and exploration of multilingual resources.  

## Characteristics

| Attribute             | Details         |
|---------------------- |---------------- |
| **Provider**          | Google DeepMind |
| **Architecture**      | Gemma 3n        |
| **Cutoff date**       | June 2024       |
| **Languages**         | 140+            |
| **Tool calling**      | ✅              |
| **Input modalities**  | Text, Image, Audio, Video |
| **Output modalities** | Text            |
| **License**           | [Gemma Terms](https://ai.google.dev/gemma/terms) |

## Available model variants

| Model variant | Parameters | Quantization | Context window | VRAM¹ | Size |
|---------------|------------|--------------|----------------|------|-------|
| `ai/gemma3n:4B-F16` | 6.9B | MOSTLY_F16 | 33K tokens | 9.32 GiB | 12.79 GB |

¹: VRAM estimated based on model characteristics.
## Use this AI model with Docker Model Runner

To run the model:

```bash
docker model pull ai/gemma3n
```

Then launch it:

```bash
docker model run ai/gemma3n
```

More details in the [Docker Model Runner documentation](https://docs.docker.com/desktop/features/model-runner/).

## Benchmark performance

| Category       | Benchmark          | 2B Value | 4B Value |
|----------------|--------------------|----------|----------|
| General        | MMLU               | 60.1     | 64.9     |
|                | DROP               | 53.9     | 60.8     |
|                | BIG-Bench Hard     | 44.3     | 52.9     |
|                | ARC-Challenge (25-shot) | 51.7     | 61.6     |
| Multilingual   | Global-MMLU        | 55.1     | 60.3     |
|                | MGSM               | 53.1     | 60.7     |
| STEM & Code    | HumanEval          | 66.5     | 75.0     |
|                | MBPP               | 56.6     | 63.6     |
|                | GPQA (Diamond)     | 24.8     | 23.7     |

## Links

- [Gemma 3n Model Page](https://ai.google.dev/gemma/docs/gemma-3n)
- [Responsible AI Toolkit](https://ai.google.dev/responsible)
