# Nomic Embed Text

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/nomic-280x184-overview.svg)

Nomic Embed Text v1 is an open‑source, fully auditable text embedding model with an 8192‑token context window. 
It outperforms OpenAI Ada‑002 and text‑embedding‑3‑small on various embedding benchmarks while providing open weights, training code, and data under an Apache‑2 license.

## Intended uses

Nomic Embed Text v1 is designed for applications requiring high‑quality embeddings over very long contexts:

- **Semantic search and retrieval**: Excellent for retrieval‑augmented generation (RAG), clustering, and information retrieval tasks using long documents.
- **Clustering and classification**: Embeddings can be used downstream for clustering, classification, and data visualization.
- **Auditable, open embedding pipelines**: Provides full transparency with open data, code, and model weights—ideal for enterprise and research use where auditability matters.


## Characteristics

| Attribute             | Details                                                                                                                                                   |
|-----------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Provider**          | Nomic AI                                                                                                                                                  |
| **Architecture**      | Transformer-based encoder, initialized from a BERT-style model (Nomic-BERT‑2048) with rotary embeddings, SwiGLU activations, and long‑context adaptations |
| **Cutoff date**       | -                                                                                                                                                         |
| **Languages**         | English                                                                                                                                                   |
| **Tool calling**      | ❌                                                                                                                                                         |
| **Input modalities**  | Text (tokens up to 8192 sequence length)                                                                                                                  |
| **Output modalities** | Embedding vectors                                                                                                                                         |
| **License**           | Apache 2.0                                                                                                                                                |

## Available model variants

| Model variant                                                                | Parameters | Quantization | Context window | VRAM¹    | Size      |
|------------------------------------------------------------------------------|------------|--------------|----------------|----------|-----------|
| `ai/nomic-embed-text-v1.5:latest`<br><br>`ai/nomic-embed-text-v1.5:137M-F16` | 137M       | MOSTLY_F16   | 2K tokens      | 0.51 GiB | 260.87 MB |
| `ai/nomic-embed-text-v1.5:137M-F16`                                          | 137M       | MOSTLY_F16   | 2K tokens      | 0.51 GiB | 260.87 MB |

¹: VRAM estimated based on model characteristics.

> `latest` → `137M-F16`

## Use this AI model with Docker Model Runner

First, pull the model:

```bash
docker model pull ai/nomic-embed-text-v1.5
```

Then run the model:

```bash
url --location 'http://localhost:12434/engines/llama.cpp/v1/embeddings' \
--header 'Content-Type: application/json' \
--data '{
    "model": "ai/nomic-embed-text-v1.5",
    "input": "hello world!"
  }'
```

## Considerations

- While performance is strong on MTEB and LoCo benchmarks, on the Jina Long Context Benchmark it does not outperform closed-source models like Ada‑002 or text‑embedding‑3‑small.
- Best suited for applications needing open-source, very long‑context embeddings with full reproducibility.

## Links
- https://www.nomic.ai/blog/posts/nomic-embed-text-v1
