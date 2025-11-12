# Codebase Embeddings Demo

This demo application enables semantic search over your codebase using AI embeddings. It allows you to ask natural language questions like "Where is the model packaging code?" and get relevant code snippets with similarity scores.

## Overview

The demo consists of:
- **Indexer**: Walks the codebase, generates embeddings for all source files
- **Search Engine**: Uses cosine similarity to find relevant code
- **Web UI**: Interactive interface for searching the codebase
- **REST API**: Backend server for search and indexing operations

## Features

- 🔍 Natural language search over entire codebase
- 📊 Similarity scores for each result
- 🎯 Smart chunking for large files (respects function boundaries in Go)
- 🚫 Respects .gitignore patterns
- 📈 Progress tracking during indexing
- 🔄 Rebuild index from UI
- 💾 Persistent storage (JSON file)

## Prerequisites

Before running this demo, you need:

1. **Node.js** (version 18 or higher)
2. **Docker Model Runner** with the embedding model loaded
3. **The embedding model** (`ai/qwen3-embedding:0.6B-F16`)

### Pull the Embedding Model

```bash
# Pull the Qwen3 embedding model
docker model pull ai/qwen3-embedding

# Verify it's available
docker model list
```

## Installation

1. **Navigate to the demo directory:**
   ```bash
   cd demos/embeddings
   ```

2. **Install dependencies:**
   ```bash
   npm install
   ```

## Usage

### Step 1: Generate the Embeddings Index

Before you can search, you need to index the codebase:

```bash
npm run index
```

This will:
- Scan all source files in the project (respecting .gitignore)
- Generate embeddings for each file/chunk
- Save the index to `embeddings-index.json`

**Note**: Indexing may take 5-15 minutes depending on the size of your codebase. Progress will be displayed in the console.

### Step 2: Start the Server

```bash
npm start
```

The server will start on `http://localhost:3000`

### Step 3: Open the Web Interface

Open your browser and navigate to:
```
http://localhost:3000
```

## Using the Search Interface

1. **Check Index Status**: The status bar shows index information (files indexed, embeddings count, last updated)

2. **Enter Your Query**: Type a natural language question or keywords
   - Example: "Where is the model packaging code?"
   - Example: "GPU memory handling implementation"
   - Example: "How does distribution client work?"

3. **View Results**: Results are ranked by similarity score
   - File path and line numbers
   - Similarity percentage
   - Code snippet preview

4. **Try Example Queries**: Click on any example query to quickly test the search

5. **Rebuild Index**: Click "Rebuild Index" to regenerate embeddings (e.g., after code changes)

## API Reference

### Search Endpoint

**POST** `/api/search`

Search the codebase with a natural language query.

**Request:**
```json
{
  "query": "Where is the model packaging code?",
  "topK": 10
}
```

**Response:**
```json
{
  "query": "Where is the model packaging code?",
  "topK": 10,
  "count": 3,
  "results": [
    {
      "filePath": "cmd/cli/commands/package.go",
      "chunkId": 0,
      "content": "package commands\n\nimport (\n...",
      "startLine": 1,
      "endLine": 50,
      "fileType": ".go",
      "similarity": 0.8542
    }
  ]
}
```

### Index Status

**GET** `/api/index/status`

Get information about the current index.

**Response:**
```json
{
  "exists": true,
  "size": 15728640,
  "sizeHuman": "15 MB",
  "modified": "2024-01-15T10:30:00.000Z",
  "metadata": {
    "projectRoot": "/path/to/project",
    "model": "ai/qwen3-embedding:0.6B-F16",
    "totalFiles": 150,
    "totalEmbeddings": 1250,
    "generatedAt": "2024-01-15T10:30:00.000Z",
    "version": "1.0"
  }
}
```

### Metadata

**GET** `/api/metadata`

Get index metadata.

### Rebuild Index

**POST** `/api/index/rebuild`

Trigger background indexing process.

## CLI Usage

You can also search from the command line:

```bash
# Search with default settings (top 10 results)
node search.js "model packaging code"

# Specify number of results
node search.js "GPU memory" 5
```

## Configuration

You can modify these settings in the respective files:

### indexer.js Configuration

```javascript
const CONFIG = {
  projectRoot: path.resolve(__dirname, '../..'),
  embeddingsAPI: 'http://localhost:12434/engines/llama.cpp/v1/embeddings',
  model: 'ai/qwen3-embedding:0.6B-F16',
  maxChunkSize: 100, // tokens per chunk
  batchSize: 5, // files to process in parallel
  fileExtensions: ['.go'],
};
```

### search.js Configuration

```javascript
const CONFIG = {
  embeddingsAPI: 'http://localhost:12434/engines/llama.cpp/v1/embeddings',
  model: 'ai/qwen3-embedding:0.6B-F16',
  defaultTopK: 10,
  similarityThreshold: 0.5, // minimum similarity score
};
```

## How It Works

### 1. File Collection
- Reads `.gitignore` to respect ignore patterns
- Filters by file extension (Go, JavaScript, Markdown, etc.)
- Excludes directories like `node_modules`, `vendor`, `build`

### 2. Chunking Strategy
- Files under 100 tokens: kept as single chunk
- Go files: split at function boundaries
- Other files: split by line count
- Maintains line number references for each chunk

### 3. Embedding Generation
- Each chunk is sent to the embedding API
- Returns a high-dimensional vector (typically 768 or 1024 dimensions)
- Vectors capture semantic meaning of the code

### 4. Search Process
- User query is converted to an embedding vector
- Cosine similarity calculated between query and all chunks
- Results sorted by similarity (highest first)
- Top K results returned

### 5. Similarity Calculation
Uses cosine similarity formula:
```
similarity = (A · B) / (||A|| × ||B||)
```

Where:
- A = query embedding vector
- B = code chunk embedding vector
- Range: 0 to 1 (higher = more similar)

## Additional Resources

- [Docker Model Runner Documentation](https://docs.docker.com/ai/model-runner/)
- [Embedding Models on Docker Hub](https://hub.docker.com/r/ai)
- [Cosine Similarity Explanation](https://en.wikipedia.org/wiki/Cosine_similarity)
