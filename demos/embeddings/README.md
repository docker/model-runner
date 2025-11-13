# Codebase Embeddings Demo

This demo application enables semantic search over your codebase using AI embeddings. Ask natural language questions like "Where is the model packaging code?" and get relevant code snippets with similarity scores.

## Quick Start (Recommended) ⚡

The demo automatically downloads a pre-generated embeddings index from a [GitHub Gist](https://gist.githubusercontent.com/ilopezluna/235518ab315c23275c90c5cddb1375b8/raw/df541734b3f4843f99b8ce34f4cee240addde08f/embeddings-index.json) on first run, so you can start searching immediately without waiting for indexing.

### Prerequisites

- **Node.js** (version 18 or higher)
- **Docker Model Runner** running on port 12434

### Installation

```bash
# Pull ai/qwen3-embedding:0.6B-F16
docker model pull ai/qwen3-embedding:0.6B-F16

# Navigate to the demo directory
cd demos/embeddings

# Install dependencies
npm install

# Start the server
npm start
```

The server will automatically:
1. Check if an embeddings index exists locally
2. If not found, download the pre-generated index from the Gist
3. Load the index and start the server
4. Open your browser to http://localhost:3000

**That's it!** You can now search the codebase with natural language queries.

### About the Pre-generated Index

The pre-generated embeddings index:
- Contains embeddings for the entire Docker Model Runner codebase
- Generated using the `ai/qwen3-embedding:0.6B-F16` model
- Hosted on GitHub Gist for easy access

**Note**: The pre-generated index represents a snapshot of the codebase. If you've made local code changes and want search results to reflect them, see the "Generate Your Own Index" section below.

---

## Generate Your Own Index 🔨

Want to generate fresh embeddings for your local codebase?

```bash
# Navigate to demo directory
cd demos/embeddings

# Run the indexer (takes ~20 minutes)
npm run index
```

This will:
1. Scan all source files in the project (respecting .gitignore)
2. Generate embeddings for each file/chunk
3. Save the index to `embeddings-index.json`

After indexing completes, start the server:
```bash
npm start
```

---

## Additional Resources

- [Docker Model Runner Documentation](https://docs.docker.com/ai/model-runner/)
- [Embedding Models on Docker Hub](https://hub.docker.com/u/ai?page=1&search=embed)
- [Pre-generated Index Gist](https://gist.githubusercontent.com/ilopezluna/235518ab315c23275c90c5cddb1375b8/raw/df541734b3f4843f99b8ce34f4cee240addde08f/embeddings-index.json)
- [Cosine Similarity Explanation](https://en.wikipedia.org/wiki/Cosine_similarity)
