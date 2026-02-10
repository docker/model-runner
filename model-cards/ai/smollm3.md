# SmolLM3

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/hugginfface-280x184-overview@2x.svg)

SmolLM3 is a compact 3.1B parameter language model designed for efficient on-device deployment while maintaining strong performance across a wide range of language tasks. Building on the success of the SmolLM series, SmolLM3 delivers improved instruction following, reasoning capabilities, and knowledge retention in a lightweight package. The model is optimized for chat assistants, text processing, and various natural language understanding tasks.

## Intended uses

SmolLM3 is designed for:

- **Chat assistants and conversational AI**
- **Text summarization and rewriting**
- **Question answering and knowledge retrieval**
- **Code assistance and text generation**
- **On-device AI applications**

## Characteristics

| Attribute             | Details       |
|---------------------- |---------------|
| **Provider**          | Hugging Face  |
| **Architecture**      | SmolLM3       |
| **Cutoff date**       | October 2024  |
| **Languages**         | English       |
| **Tool calling**      | ✅            |
| **Input modalities**  | Text          |
| **Output modalities** | Text          |
| **License**           | [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) |

## Available model variants

| Model variant | Parameters | Quantization | Context window | VRAM¹ | Size |
|---------------|------------|--------------|----------------|------|-------|
| `ai/smollm3:latest`<br><br>`ai/smollm3:Q4_K_M` | 3.1B | MOSTLY_Q4_K_M | 66K tokens | 2.45 GiB | 1.78 GB |
| `ai/smollm3:F16` | 3.1B | MOSTLY_F16 | 66K tokens | 6.40 GiB | 5.73 GB |
| `ai/smollm3:Q8_0` | 3.1B | MOSTLY_Q8_0 | 66K tokens | 3.72 GiB | 3.04 GB |

¹: VRAM estimated based on model characteristics.

> `latest` → `Q4_K_M`

## Use this AI model with Docker Model Runner

First, pull the model:

```bash
docker model pull ai/smollm3
```

Then run the model:

```bash
docker model run ai/smollm3
```

For more information on Docker Model Runner, [explore the documentation](https://docs.docker.com/desktop/features/model-runner/).

## Links

- [SmolLM3 GGUF on Hugging Face](https://huggingface.co/ggml-org/SmolLM3-3B-GGUF)
- [SmolLM3 Original Model on Hugging Face](https://huggingface.co/HuggingFaceTB/SmolLM3-3B)
- [SmolLM3 Blog Post](https://huggingface.co/blog/smollm3)
- [SmolLM Series Research](https://huggingface.co/collections/HuggingFaceTB/smollm-6695016cad7167254ce15966)