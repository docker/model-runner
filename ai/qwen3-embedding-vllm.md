# Qwen3-Embedding

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/qwen-280x184-overview@2x.svg)

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
| **Output modalities** | Text embeddings                                                                                                                                                                                                |
| **License**           | Apache 2.0                                                                                                                                                                                          |

---

## 🐳 Using this model with Docker Model Runner

First, pull the model:

```bash
docker model pull ai/qwen3-embedding-vllm
```

Then run the model:

```bash
curl --location 'http://localhost:12434/engines/vllm/v1/embeddings' \
--header 'Content-Type: application/json' \
--data '{
    "model": "ai/qwen3-embedding-vllm",
    "input": "hello world!"
  }'
```

For more information, check out the [Docker Model Runner docs](https://docs.docker.com/desktop/features/model-runner/).

---

### MTEB (Multilingual)

| Model                          | Size | Mean (Task) | Mean (Type) | Bitext Mining | Class. | Clust. | Inst. | Retri. | Multi. Class. | Pair. Class. | Rerank Retri. | STS  |
|--------------------------------|------|-------------|-------------|-------------|--------|--------|-------|--------|----------------|--------------|--------------|------|
| NV-Embed-v2                    | 7B   | 56.29       | 49.58       | 57.84       | 57.29  | 40.80  | 1.04  | 18.63  | 78.94          | 63.82        | 56.72        | 71.10 |
| GritLM-7B                      | 7B   | 60.92       | 53.74       | 70.53       | 61.83  | 49.75  | 3.45  | 22.77  | 79.94          | 63.78        | 58.31        | 73.33 |
| BGE-M3                         | 0.6B | 59.56       | 52.18       | 79.11       | 60.35  | 40.88  | -3.11 | 20.1   | 80.76          | 62.79        | 54.60        | 74.12 |
| multilingual-e5-large-instruct | 0.6B | 63.22       | 55.08       | 80.13       | 64.94  | 50.75  | -0.40 | 22.91  | 80.86          | 62.61        | 57.12        | 76.81 |
| gte-Qwen2-1.5B-instruct        | 1.5B | 59.45       | 52.69       | 62.51       | 58.32  | 52.05  | 0.74  | 24.02  | 81.58          | 62.58        | 60.78        | 71.61 |
| gte-Qwen2-7B-Instruct          | 7B   | 62.51       | 55.93       | 73.92       | 61.55  | 52.77  | 4.94  | 25.48  | 85.13          | 65.55        | 60.08        | 73.98 |
| text-embedding-3-large         | –    | 58.93       | 51.41       | 62.17       | 60.27  | 46.89  | -2.68 | 22.03  | 79.17          | 63.89        | 59.27        | 71.68 |
| Cohere-embed-multilingual-v3.0 | –    | 61.12       | 53.23       | 70.50       | 62.95  | 46.89  | -1.89 | 22.74  | 79.88          | 64.07        | 59.16        | 74.80 |
| gemini-embedding-exp-03-07     | –    | 68.37       | 59.59       | 79.28       | 71.82  | 54.59  | 5.18  | 29.16  | 83.63          | 65.58        | 67.71        | 79.40 |
| **Qwen3-Embedding-0.6B**       | 0.6B | 64.33       | 56.00       | 72.22       | 66.83  | 52.33  | 5.09  | 24.59  | 80.83          | 61.41        | 64.64        | 76.17 |
| **Qwen3-Embedding-4B**         | 4B   | 69.45       | 60.86       | 79.36       | 72.33  | 57.15  | 11.56 | 26.77  | 85.05          | 65.08        | 69.60        | 80.86 |
| **Qwen3-Embedding-8B**         | 8B   | 70.58       | 61.69       | 80.89       | 74.00  | 57.65  | 10.06 | 28.66  | 86.40          | 65.63        | 70.88        | 81.08 |
> Note: For compared models, the scores are retrieved from MTEB online leaderboard on May 24th, 2025.

---

## 🔗 Links

- [Qwen3-Embedding 0.6B](https://huggingface.co/Qwen/Qwen3-Embedding-0.6B)
- [Qwen3-Embedding 4B](https://huggingface.co/Qwen/Qwen3-Embedding-4B)
- [Qwen3-Embedding 8B](https://huggingface.co/Qwen/Qwen3-Embedding-8B)
