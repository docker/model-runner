# Moondream 2

## Description
Moondream is an open-source visual language model that understands images using simple text prompts. It's fast and wildly capable.

## Characteristics

| Attribute             | Details                                                   |
|-----------------------|-----------------------------------------------------------|
| **Provider**          | moondream.ai                                              |
| **Architecture**      | phi2                                                      |
| **Cutoff date**       | -                                                         |
| **Languages**         | English                                                   |
| **Tool calling**      | ❌                                                         |
| **Input modalities**  | Text, Image                                               |
| **Output modalities** | Text                                                      |
| **License**           | [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) |

## Available model variants

| Model variant                                                                      | Parameters | Quantization | Context window | VRAM¹    | Size    |
|------------------------------------------------------------------------------------|------------|--------------|----------------|----------|---------|
| `ai/moondream2:1.5B`<br><br>`ai/moondream2:1.5B-F16`<br><br>`ai/moondream2:latest` | 1.42 B     | MOSTLY_F16   | 2K tokens      | 3.72 GiB | 2.64 GB |

¹: VRAM estimated based on model characteristics.

> `latest` → `1.5B`

## Use this AI model with Docker Model Runner

```bash
docker model run ai/moondream2
```

## Links
- https://github.com/vikhyat/moondream
- https://moondream.ai/
- https://moondream.ai/c/playground
