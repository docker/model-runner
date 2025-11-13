const fs = require('fs').promises;
const path = require('path');
const { glob } = require('glob');
const ignore = require('ignore');

// Configuration
const CONFIG = {
  projectRoot: path.resolve(__dirname, '../..'),
  embeddingsAPI: 'http://localhost:12434/engines/llama.cpp/v1/embeddings',
  model: 'ai/qwen3-embedding:0.6B-F16',
  maxChunkSize: 300, // Max tokens per chunk
  outputFile: path.join(__dirname, 'embeddings-index.json'),
  batchSize: 5, // Process N files at a time to avoid overwhelming API
  fileExtensions: ['.go'], // Only index Go files
};

class CodebaseIndexer {
  constructor() {
    this.embeddings = [];
    this.processedFiles = 0;
    this.totalFiles = 0;
    this.startTime = Date.now();
  }

  async loadGitignore() {
    const gitignorePath = path.join(CONFIG.projectRoot, '.gitignore');
    try {
      const content = await fs.readFile(gitignorePath, 'utf8');
      const ig = ignore().add(content);
      
      // Always ignore these
      ig.add(['node_modules', '.git', 'embeddings-index.json', 'models-store', 'model-store']);
      
      return ig;
    } catch (error) {
      console.warn('No .gitignore found, using defaults');
      return ignore().add(['node_modules', '.git', 'embeddings-index.json']);
    }
  }

  async collectFiles(ig) {
    console.log('Collecting source files...');
    
    const allFiles = await glob('**/*', {
      cwd: CONFIG.projectRoot,
      nodir: true,
      dot: false,
    });

    const sourceFiles = allFiles.filter(file => {
      // Check if ignored
      if (ig.ignores(file)) {
        return false;
      }

      // Check if valid extension
      const ext = path.extname(file);
      if (!CONFIG.fileExtensions.includes(ext)) {
        return false;
      }

      // Skip certain directories even if not in gitignore
      const parts = file.split(path.sep);
      return !parts.some(p => ['vendor', 'build', 'dist', '.idea'].includes(p));
    });

    this.totalFiles = sourceFiles.length;
    console.log(`Found ${this.totalFiles} source files to index`);
    return sourceFiles;
  }

  async readFileContent(filePath) {
    const fullPath = path.join(CONFIG.projectRoot, filePath);
    try {
        return await fs.readFile(fullPath, 'utf8');
    } catch (error) {
      console.error(`Error reading ${filePath}:`, error.message);
      return null;
    }
  }

  chunkContent(content, filePath) {
    const maxChars = CONFIG.maxChunkSize * 4; // Rough token estimate
    
    // If content is small enough, return as single chunk
    if (content.length <= maxChars) {
      return [{
        content: content,
        startLine: 1,
        endLine: content.split('\n').length,
      }];
    }

    // For Go files, try to split by functions
    const ext = path.extname(filePath);
    if (ext === '.go') {
      return this.chunkGoFile(content, maxChars);
    }

    // For other files, split by paragraphs/sections
    return this.chunkByLines(content, maxChars);
  }

  chunkGoFile(content, maxChars) {
    const lines = content.split('\n');
    const chunks = [];
    let currentChunk = [];
    let startLine = 1;
    let currentSize = 0;

    for (let i = 0; i < lines.length; i++) {
      const line = lines[i];
      const lineSize = line.length;

      // Check for function boundaries (matches both functions and methods with receivers)
      const isFunctionStart = /^func\s*(\([^\)]*\)\s*)?\w+/.test(line.trim());
      
      if (isFunctionStart && currentChunk.length > 0 && currentSize > maxChars * 0.3) {
        // Save current chunk before starting new function
        chunks.push({
          content: currentChunk.join('\n'),
          startLine: startLine,
          endLine: i,
        });
        currentChunk = [line];
        startLine = i + 1;
        currentSize = lineSize;
      } else if (currentSize + lineSize > maxChars && currentChunk.length > 0) {
        // Chunk is too large, save it
        chunks.push({
          content: currentChunk.join('\n'),
          startLine: startLine,
          endLine: i,
        });
        currentChunk = [line];
        startLine = i + 1;
        currentSize = lineSize;
      } else {
        currentChunk.push(line);
        currentSize += lineSize;
      }
    }

    // Add remaining chunk
    if (currentChunk.length > 0) {
      chunks.push({
        content: currentChunk.join('\n'),
        startLine: startLine,
        endLine: lines.length,
      });
    }

    return chunks;
  }

  chunkByLines(content, maxChars) {
    const lines = content.split('\n');
    const chunks = [];
    let currentChunk = [];
    let startLine = 1;
    let currentSize = 0;

    for (let i = 0; i < lines.length; i++) {
      const line = lines[i];
      const lineSize = line.length;

      if (currentSize + lineSize > maxChars && currentChunk.length > 0) {
        chunks.push({
          content: currentChunk.join('\n'),
          startLine: startLine,
          endLine: i,
        });
        currentChunk = [line];
        startLine = i + 1;
        currentSize = lineSize;
      } else {
        currentChunk.push(line);
        currentSize += lineSize;
      }
    }

    if (currentChunk.length > 0) {
      chunks.push({
        content: currentChunk.join('\n'),
        startLine: startLine,
        endLine: lines.length,
      });
    }

    return chunks;
  }

  async generateEmbedding(text) {
    try {
      const response = await fetch(CONFIG.embeddingsAPI, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          model: CONFIG.model,
          input: text,
        }),
      });

      if (!response.ok) {
        throw new Error(`API responded with ${response.status}: ${await response.text()}`);
      }

      const data = await response.json();
      return data.data[0].embedding;
    } catch (error) {
      console.error('Error generating embedding:', error.message);
      return null;
    }
  }

  async processFile(filePath) {
    const content = await this.readFileContent(filePath);
    if (!content) {
      return [];
    }

    const chunks = this.chunkContent(content, filePath);
    const results = [];

    for (let i = 0; i < chunks.length; i++) {
      const chunk = chunks[i];
      
      // Skip very small chunks (< 50 chars)
      if (chunk.content.trim().length < 50) {
        continue;
      }

      const embedding = await this.generateEmbedding(chunk.content);
      if (!embedding) {
        continue;
      }

      results.push({
        filePath: filePath,
        chunkId: i,
        content: chunk.content,
        startLine: chunk.startLine,
        endLine: chunk.endLine,
        fileType: path.extname(filePath),
        embedding: embedding,
        timestamp: new Date().toISOString(),
      });

      // Small delay to avoid overwhelming API
      await new Promise(resolve => setTimeout(resolve, 100));
    }

    return results;
  }

  async processBatch(files) {
    const promises = files.map(file => this.processFile(file));
    const results = await Promise.all(promises);
    
    // Flatten results
    const allEmbeddings = results.flat();
    this.embeddings.push(...allEmbeddings);
    this.processedFiles += files.length;

    // Progress update
    const elapsed = ((Date.now() - this.startTime) / 1000).toFixed(1);
    const progress = ((this.processedFiles / this.totalFiles) * 100).toFixed(1);
    const embeddingCount = this.embeddings.length;
    
    console.log(
      `Progress: ${this.processedFiles}/${this.totalFiles} files (${progress}%) | ` +
      `${embeddingCount} embeddings | ${elapsed}s elapsed`
    );
  }

  async index() {
    console.log('Starting codebase indexing...');
    console.log(`Project root: ${CONFIG.projectRoot}`);
    console.log(`Model: ${CONFIG.model}`);
    console.log(`API: ${CONFIG.embeddingsAPI}`);
    console.log();

    // Load .gitignore
    const ig = await this.loadGitignore();

    // Collect files
    const files = await this.collectFiles(ig);

    // Process in batches
    for (let i = 0; i < files.length; i += CONFIG.batchSize) {
      const batch = files.slice(i, i + CONFIG.batchSize);
      await this.processBatch(batch);
    }

    // Save results
    console.log();
    console.log('Saving embeddings index...');
    await fs.writeFile(
      CONFIG.outputFile,
      JSON.stringify({
        metadata: {
          projectRoot: CONFIG.projectRoot,
          model: CONFIG.model,
          totalFiles: this.processedFiles,
          totalEmbeddings: this.embeddings.length,
          generatedAt: new Date().toISOString(),
          version: '1.0',
        },
        embeddings: this.embeddings,
      }, null, 2)
    );

    const elapsed = ((Date.now() - this.startTime) / 1000).toFixed(1);
    console.log();
    console.log('✓ Indexing complete!');
    console.log(`  Files processed: ${this.processedFiles}`);
    console.log(`  Embeddings generated: ${this.embeddings.length}`);
    console.log(`  Time taken: ${elapsed}s`);
    console.log(`  Output: ${CONFIG.outputFile}`);
  }
}

// Run indexer if executed directly
if (require.main === module) {
  const indexer = new CodebaseIndexer();
  indexer.index().catch(error => {
    console.error('Fatal error:', error);
    process.exit(1);
  });
}

module.exports = CodebaseIndexer;
