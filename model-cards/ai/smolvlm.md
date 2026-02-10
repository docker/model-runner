# SmolVLM

## Description
SmolVLM is a lightweight multimodal model designed to analyze video content. The model processes videos, images, and text inputs to generate text outputs - whether answering questions about media files, comparing visual content, or transcribing text from images. Despite its compact size, requiring only 1.8GB of GPU RAM for video inference, it delivers robust performance on complex multimodal tasks. This efficiency makes it particularly well-suited for on-device applications where computational resources may be limited.

## Characteristics

| Attribute             | Details                                                   |
|-----------------------|-----------------------------------------------------------|
| **Provider**          | Hugging Face                                              |
| **Architecture**      | Llama                                                     |
| **Cutoff date**       | -                                                         |
| **Languages**         | English                                                   |
| **Tool calling**      | ❌                                                         |
| **Input modalities**  | Text, Image                                               |
| **Output modalities** | Text                                                      |
| **License**           | [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) |

## Available model variants

| Model variant                                                             | Parameters | Quantization | Context window | VRAM¹    | Size      |
|---------------------------------------------------------------------------|------------|--------------|----------------|----------|-----------|
| `ai/smolvlm:500M`<br><br>`ai/smolvlm:500M-F16`<br><br>`ai/smolvlm:latest` | 500M       | MOSTLY_F16   | 8K tokens      | 1.15 GiB | 780.71 MB |
| `ai/smolvlm:500M-Q8_0`                                                    | 500M       | MOSTLY_Q8_0  | 8K tokens      | 0.84 GiB | 414.86 MB |

¹: VRAM estimated based on model characteristics.

> `latest` → `500M`

## Use this AI model with Docker Model Runner

```bash
docker model run ai/smolvlm
```

## Links
- https://huggingface.co/HuggingFaceTB/SmolVLM2-500M-Video-Instruct
- https://huggingface.co/blog/smolvlm
- https://huggingface.co/ggml-org/SmolVLM-500M-Instruct-GGUF
