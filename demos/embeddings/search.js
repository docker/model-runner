const fs = require('fs').promises;
const path = require('path');

// Configuration
const CONFIG = {
  embeddingsAPI: 'http://localhost:12434/engines/llama.cpp/v1/embeddings',
  model: 'ai/qwen3-embedding:0.6B-F16',
  indexFile: path.join(__dirname, 'embeddings-index.json'),
  defaultTopK: 10,
  similarityThreshold: 0.5,
};

class SemanticSearch {
  constructor() {
    this.index = null;
    this.embeddings = [];
  }

  async loadIndex() {
    try {
      const data = await fs.readFile(CONFIG.indexFile, 'utf8');
      this.index = JSON.parse(data);
      this.embeddings = this.index.embeddings;
      console.log(`Loaded ${this.embeddings.length} embeddings from index`);
      return true;
    } catch (error) {
      console.error('Error loading index:', error.message);
      return false;
    }
  }

  async generateQueryEmbedding(query) {
    try {
      const response = await fetch(CONFIG.embeddingsAPI, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          model: CONFIG.model,
          input: query,
        }),
      });

      if (!response.ok) {
        throw new Error(`API responded with ${response.status}: ${await response.text()}`);
      }

      const data = await response.json();
      return data.data[0].embedding;
    } catch (error) {
      console.error('Error generating query embedding:', error.message);
      throw error;
    }
  }

  cosineSimilarity(vecA, vecB) {
    if (vecA.length !== vecB.length) {
      console.warn('cosineSimilarity: Vectors must have the same length. Skipping invalid result.');
      return null;
    }

    // Validate that all elements are finite numbers
    for (let i = 0; i < vecA.length; i++) {
      if (!Number.isFinite(vecA[i]) || !Number.isFinite(vecB[i])) {
        throw new Error('Vectors must not contain NaN or infinite values');
      }
    }

    let dotProduct = 0;
    let normA = 0;
    let normB = 0;

    for (let i = 0; i < vecA.length; i++) {
      dotProduct += vecA[i] * vecB[i];
      normA += vecA[i] * vecA[i];
      normB += vecB[i] * vecB[i];
    }

    normA = Math.sqrt(normA);
    normB = Math.sqrt(normB);

    if (normA === 0 || normB === 0) {
      return 0;
    }

    return dotProduct / (normA * normB);
  }

  async search(query, topK = CONFIG.defaultTopK) {
    if (!this.index) {
      await this.loadIndex();
    }

    if (this.embeddings.length === 0) {
      throw new Error('No embeddings found in index. Please run the indexer first.');
    }

    // Generate embedding for the query
    console.log(`Generating embedding for query: "${query}"`);
    const queryEmbedding = await this.generateQueryEmbedding(query);

    // Calculate similarities
    console.log('Calculating similarities...');
    const results = this.embeddings.map(item => {
      const similarity = this.cosineSimilarity(queryEmbedding, item.embedding);
      return {
        filePath: item.filePath,
        chunkId: item.chunkId,
        content: item.content,
        startLine: item.startLine,
        endLine: item.endLine,
        fileType: item.fileType,
        similarity: similarity,
      };
    });

    // Sort by similarity (highest first) and filter by threshold
    const filtered = results
      .filter(r => r.similarity >= CONFIG.similarityThreshold)
      .sort((a, b) => b.similarity - a.similarity)
      .slice(0, topK);

    console.log(`Found ${filtered.length} results above threshold ${CONFIG.similarityThreshold}`);
    return filtered;
  }

  getMetadata() {
    return this.index ? this.index.metadata : null;
  }

  formatResults(results) {
    return results.map((r, idx) => {
      const preview = r.content.length > 200 
        ? r.content.substring(0, 200) + '...' 
        : r.content;
      
      return {
        rank: idx + 1,
        file: r.filePath,
        lines: `${r.startLine}-${r.endLine}`,
        similarity: (r.similarity * 100).toFixed(2) + '%',
        preview: preview,
      };
    });
  }
}

// CLI usage
if (require.main === module) {
  const query = process.argv[2];
  const topK = parseInt(process.argv[3]) || 10;

  if (!query) {
    console.error('Usage: node search.js "your search query" [topK]');
    console.error('Example: node search.js "model packaging" 5');
    process.exit(1);
  }

  const search = new SemanticSearch();
  
  search.search(query, topK)
    .then(results => {
      console.log('\nSearch Results:');
      console.log('===============\n');
      
      if (results.length === 0) {
        console.log('No results found.');
        return;
      }

      results.forEach((r, idx) => {
        console.log(`${idx + 1}. ${r.filePath} (lines ${r.startLine}-${r.endLine})`);
        console.log(`   Similarity: ${(r.similarity * 100).toFixed(2)}%`);
        console.log(`   Preview: ${r.content.substring(0, 150).replace(/\n/g, ' ')}...`);
        console.log();
      });
    })
    .catch(error => {
      console.error('Search failed:', error);
      process.exit(1);
    });
}

module.exports = SemanticSearch;
