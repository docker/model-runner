# Granite Docling

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/ibm-280x184-overview.svg)

## Description
Granite Docling is a multimodal Image-Text-to-Text model engineered for efficient document conversion. It preserves the core features of Docling while maintaining seamless integration with [Docling Documents](https://docling-project.github.io/docling) to ensure full compatibility.

## Characteristics

| Attribute             | Details                                                                          |
|-----------------------|----------------------------------------------------------------------------------|
| **Provider**          | IBM Research                                                                     |
| **Architecture**      | Based on Idefics2-8B; vision encoder = siglip-base-patch16-512; LLM = Granite 165M |
| **Cutoff date**       | -                                                                                |
| **Languages**         | English (with experimental support for Japanese, Arabic, Chinese)                |
| **Tool calling**      | ❌                                                                                |
| **Input modalities**  | Text, Image                                                                      |
| **Output modalities** | Text                                                                             |
| **License**           | [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0)                        |

## Available model variants

| Model variant                                                                                     | Parameters | Quantization | Context window | VRAM¹    | Size      |
|---------------------------------------------------------------------------------------------------|------------|--------------|----------------|----------|-----------|
| `ai/granite-docling:258M`<br><br>`ai/granite-docling:258M-F16`<br><br>`ai/granite-docling:latest` | 258M       | MOSTLY_F16   | 8K tokens      | 0.86 GiB | 312.88 MB |
| `ai/granite-docling:258M-Q8_0`                                                                    | 258M       | MOSTLY_Q8_0  | 8K tokens      | 0.72 GiB | 166.28 MB |

¹: VRAM estimated based on model characteristics.

> `latest` → `258M`

## Use this AI model with Docker Model Runner

```bash
docker model run ai/granite-docling
```

## Considerations

- Best suited for document conversion and extraction workflows (PDF → Markdown/HTML/structured outputs).
- Recommended to use through the Docling library or SDK for optimal integration and inference stability.
- Supports English natively; Japanese, Arabic, and Chinese support is experimental.

Granite-Docling-258M emphasizes layout fidelity and content integrity over creative or open-ended generation. It is released under Apache 2.0 and integrates seamlessly with the Docling ecosystem for structured document AI workflows.

## Links
- https://huggingface.co/ibm-granite/granite-docling-258M
- https://huggingface.co/ggml-org/granite-docling-258M-GGUF
