# Snowflake's Arctic-embed-l-v2.0

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/snowflake-280x184-overview.png)

Snowflake arctic-embed-l-v2.0 is the newest addition to the suite of embedding models Snowflake has released optimizing for retrieval performance and inference efficiency. Arctic Embed 2.0 introduces a new standard for multilingual embedding models, combining high-quality multilingual text retrieval without sacrificing performance in English. Released under the permissive Apache 2.0 license, Arctic Embed 2.0 is ideal for applications that demand reliable, enterprise-grade multilingual search and retrieval at scale.

---

## 📌 Characteristics

| Attribute             | Value           |
|-----------------------|-----------------|
| **Provider**          | Snowflake       |
| **Languages**         | 74 Languages    |
| **Tool calling**      | ❌               |
| **Input modalities**  | Text            |
| **Output modalities** | Text embeddings |
| **License**           | Apache 2.0      |

---

## 🐳 Using this model with Docker Model Runner

First, pull the model:

```bash
docker model pull ai/snowflake-arctic-embed-l-v2-vllm
```

Then run the model:

```bash
curl --location 'http://localhost:12435/engines/vllm/v1/embeddings' \
--header 'Content-Type: application/json' \
--data '{
    "model": "ai/snowflake-arctic-embed-l-v2-vllm",
    "input": "hello world!"
  }'
```

For more information, check out the [Docker Model Runner docs](https://docs.docker.com/desktop/features/model-runner/).

---

### MTEB (Multilingual)

Unlike most other open-source models, Arctic-embed-l-v2.0 excels across English (via MTEB Retrieval) and multilingual (via MIRACL and CLEF). You no longer need to support models to empower high-quality English and multilingual retrieval. All numbers mentioned below are the average NDCG@10 across the dataset being discussed.

| Model                   | # Params | # Non-embedding Params | Dimensions | BEIR (15) | MIRACL (4) | CLEF (Focused) | CLEF (Full) |
|-------------------------|----------|------------------------|------------|-----------|------------|----------------|-------------|
| snowflake-arctic-l-v2.0 | 568M     | 303M                   | 1024       | 55.6      | 55.8       | 52.9           | 54.3        |
| snowflake-arctic-m      | 109M     | 86M                    | 768        | 54.9      | 24.9       | 34.4           | 29.1        |
| snowflake-arctic-l      | 335M     | 303M                   | 1024       | 56.0      | 34.8       | 38.2           | 33.7        |
| me5 base                | 560M     | 303M                   | 1024       | 51.4      | 54.0       | 43.0           | 34.6        |
| bge-m3 (BAAI)           | 568M     | 303M                   | 1024       | 48.8      | 56.8       | 40.8           | 41.3        |
| gte (Alibaba)           | 305M     | 113M                   | 768        | 51.1      | 52.3       | 47.7           | 53.1        |

---

## 🔗 Links

- [Technical Report](https://arxiv.org/abs/2412.04506)
- [Hugging Face Model Card](https://huggingface.co/Snowflake/snowflake-arctic-embed-l-v2.0)
