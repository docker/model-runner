# Qwen3-Reranker

The Qwen3 Embedding model series is the latest proprietary model of the Qwen family, specifically designed for text embedding and ranking tasks. Building upon the dense foundational models of the Qwen3 series, it provides a comprehensive range of text embeddings and reranking models in various sizes (0.6B, 4B, and 8B). This series inherits the exceptional multilingual capabilities, long-text understanding, and reasoning skills of its foundational model. The Qwen3 Embedding series represents significant advancements in multiple text embedding and ranking tasks, including text retrieval, code retrieval, text classification, text clustering, and bitext mining.

---

## 📌 Characteristics

| Attribute             | Value                                                                                                                                                                                               |
|-----------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Provider**          | Alibaba Cloud                                                                                                                                                                                       |
| **Architecture**      | qwen3                                                                                                                                                                                               |
| **Languages**         | 119 languages from multiple families  (Indo European, Sino-Tibetan, Afro-Asiatic, Austronesian, Dravidian, Turkic, Tai-Kadai, Uralic, Austroasiatic) including others like Japanese, Basque, Haitian,... |
| **Tool calling**      | ❌                                                                                                                                                                                                    |
| **Input modalities**  | Text                                                                                                                                                                                                |
| **Output modalities** | Scores                                                                                                                                                                                                |
| **License**           | Apache 2.0                                                                                                                                                                                          |

---

## Available model variants

| Model variant | Parameters | Quantization | Context window | VRAM¹ | Size |
|---------------|------------|--------------|----------------|------|-------|
| `ai/qwen3-reranker:4B` <br><br> `ai/qwen3-reranker:latest`| 4B | F16 | 32K tokens | 8 GiB | 1.11 GB |
| `ai/qwen3-reranker:0.6B`| 0.6B | F16 | 32K tokens | 1.2 GiB | 2.32 GB |
| `ai/qwen3-reranker:8B` | 8B | F16 | 32K tokens | 16 GiB | 7.49 GB |

¹: VRAM estimated based on model characteristics.

> `latest` → `4B`

---

## 🐳 Using this model with Docker Model Runner

First, pull the model:

```bash
docker model pull ai/qwen3-reranker
```

Then run the model:

```bash
curl --location 'http://localhost:8080/engines/vllm/rerank' \
--header 'Content-Type: application/json' \
--data '{
  "model": "ai/qwen3-reranker:0.6B",
  "query": "What is the capital of France?",
  "documents": [
    "The capital of Brazil is Brasilia.",
    "The capital of France is Paris.",
    "Horses and cows are both animals."
  ]
}'
```

```bash
curl --location 'http://localhost:8080/engines/vllm/score' \
--header 'Content-Type: application/json' \
--data '{
  "model": "ai/qwen3-reranker:0.6B",
  "text_1": "ping",
  "text_2": "pong"
}'
```

For more information, check out the [Docker Model Runner docs](https://docs.docker.com/desktop/features/model-runner/).

---

## Evaluation

| Model                              | Param  | MTEB-R  | CMTEB-R | MMTEB-R | MLDR   | MTEB-Code | FollowIR |
|------------------------------------|--------|---------|---------|---------|--------|-----------|----------|
| **Qwen3-Embedding-0.6B**               | 0.6B   | 61.82   | 71.02   | 64.64   | 50.26  | 75.41     | 5.09     |
| Jina-multilingual-reranker-v2-base | 0.3B   | 58.22   | 63.37   | 63.73   | 39.66  | 58.98     | -0.68    |
| gte-multilingual-reranker-base                      | 0.3B   | 59.51   | 74.08   | 59.44   | 66.33  | 54.18     | -1.64    |
| BGE-reranker-v2-m3                 | 0.6B   | 57.03   | 72.16   | 58.36   | 59.51  | 41.38     | -0.01    |
| **Qwen3-Reranker-0.6B**                | 0.6B   | 65.80   | 71.31   | 66.36   | 67.28  | 73.42     | 5.41     |
| **Qwen3-Reranker-4B**                  | 4B   | **69.76** | 75.94   | 72.74   | 69.97  | 81.20     | **14.84** |
| **Qwen3-Reranker-8B**                  | 8B     | 69.02   | **77.45** | **72.94** | **70.19** | **81.22** | 8.05     |

---

## 🔗 Links

- [Qwen3-Reranker 0.6B](https://huggingface.co/Qwen/Qwen3-Reranker-0.6B)
- [Qwen3-Reranker 4B](https://huggingface.co/Qwen/Qwen3-Reranker-4B)
- [Qwen3-Reranker 8B](https://huggingface.co/Qwen/Qwen3-Reranker-8B)
