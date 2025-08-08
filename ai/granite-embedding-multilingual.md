# Granite Embedding Multilingual

Granite Embedding Multilingual is a 278 million parameter, encoder‑only XLM‑RoBERTa‑style dense biencoder model created by IBM, engineered to produce high‑quality multilingual text embeddings (768‑dimensional). It’s optimized for semantic similarity, retrieval, and search in 12 major languages, released under the Apache 2.0 license.

## Intended uses

Designed for fixed-length vector generation suitable for multilingual search and retrieval tasks:

- **Semantic similarity and retrieval**: Compute vector representations for efficient similarity comparisons in multilingual contexts
- **Cross-lingual information retrieval (RAG, search)**: Works across 12 languages for tasks like clustering or search
- **Enterprise-grade deployment**: Built with ethically sourced, enterprise‑friendly datasets and transparent processes

## Characteristics

| Attribute             | Details                                                                                                              |
|-----------------------|----------------------------------------------------------------------------------------------------------------------|
| **Provider**          | IBM (Granite Embedding Team)                                                                                         |
| **Architecture**      | Encoder‑only transformer, XLM‑RoBERTa‑like bi‑encoder                                                                |
| **Cutoff date**       | Released December 18, 2024:contentReference                                                                          |
| **Languages**         | Multilingual: English, German, Spanish, French, Japanese, Portuguese, Arabic, Czech, Italian, Korean, Dutch, Chinese |
| **Tool calling**      | No (not for tool‑calling; it's an embedding model)                                                                   |
| **Input modalities**  | Text (up to 512 tokens per input)                                                                                    |
| **Output modalities** | Fixed-length embedding vectors (768 dimensions)                                                                      |
| **License**           | Apache 2.0                                                                                                           |


## Available model variants

| Model variant | Parameters | Quantization | Context window | VRAM¹ | Size |
|---------------|------------|--------------|----------------|------|-------|
| `ai/granite-embedding-multilingual:latest`<br><br>`ai/granite-embedding-multilingual:278M-F16` | 278M | MOSTLY_F16 | 512 tokens | 0.19 GiB | 530.18 MB |
| `ai/granite-embedding-multilingual:278M-F16` | 278M | MOSTLY_F16 | 512 tokens | 0.19 GiB | 530.18 MB |

¹: VRAM estimated based on model characteristics.

> `latest` → `278M-F16`

## Use this AI model with Docker Model Runner

First, pull the model:

```bash
docker model pull {model_name}
```

Then run the model:

```bash
docker model run {model_name}
```

## Considerations

- Context is limited to 512 tokens—longer inputs need truncation or chunking.
- Performance is strong on multilingual.
- Built following IBM’s AI ethics guidelines, with transparent dataset governance and licensing.

## Links
- https://www.ibm.com/architectures/product-guides/granite-embedding
- https://huggingface.co/ibm-granite/granite-embedding-278m-multilingual
