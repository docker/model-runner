const express = require('express');
const cors = require('cors');
const path = require('path');
const fs = require('fs').promises;
const SemanticSearch = require('./search');
const CodebaseIndexer = require('./indexer');

const app = express();
const PORT = process.env.PORT || 3000;

// Configuration
const PREGENERATED_INDEX_URL = 'https://gist.githubusercontent.com/ilopezluna/235518ab315c23275c90c5cddb1375b8/raw/df541734b3f4843f99b8ce34f4cee240addde08f/embeddings-index.json';
const INDEX_PATH = path.join(__dirname, 'embeddings-index.json');

// Middleware
app.use(cors());
app.use(express.json());
app.use(express.static(__dirname));

// Initialize search instance
const search = new SemanticSearch();

// Health check
app.get('/health', (req, res) => {
  res.json({ status: 'ok', timestamp: new Date().toISOString() });
});

// Get index metadata
app.get('/api/metadata', async (req, res) => {
  try {
    await search.loadIndex();
    const metadata = search.getMetadata();
    
    if (!metadata) {
      return res.status(404).json({ 
        error: 'No index found. Please run the indexer first.' 
      });
    }

    res.json(metadata);
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// Search endpoint
app.post('/api/search', async (req, res) => {
  try {
    const { query, topK = 10 } = req.body;

    if (!query || typeof query !== 'string') {
      return res.status(400).json({ error: 'Query is required and must be a string' });
    }

    if (query.trim().length === 0) {
      return res.status(400).json({ error: 'Query cannot be empty' });
    }

    console.log(`Received search request: "${query}" (topK: ${topK})`);
    
    const results = await search.search(query, topK);
    
    res.json({
      query: query,
      topK: topK,
      count: results.length,
      results: results,
    });
  } catch (error) {
    console.error('Search error:', error);
    res.status(500).json({ error: error.message });
  }
});

// Index status endpoint
app.get('/api/index/status', async (req, res) => {
  try {
    const fs = require('fs').promises;
    const indexPath = path.join(__dirname, 'embeddings-index.json');
    
    try {
      const stats = await fs.stat(indexPath);
      const data = await fs.readFile(indexPath, 'utf8');
      const index = JSON.parse(data);
      
      res.json({
        exists: true,
        size: stats.size,
        sizeHuman: formatBytes(stats.size),
        modified: stats.mtime,
        metadata: index.metadata,
      });
    } catch (error) {
      if (error.code === 'ENOENT') {
        res.json({ exists: false });
      } else {
        throw error;
      }
    }
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// Trigger indexing endpoint (for UI)
app.post('/api/index/rebuild', async (req, res) => {
  try {
    // Set a timeout for this long-running operation
    req.setTimeout(30 * 60 * 1000); // 30 minutes

    res.json({ 
      message: 'Indexing started. This may take several minutes. Check the server logs for progress.',
      note: 'The server will continue processing in the background.'
    });

    // Run indexing in background
    const indexer = new CodebaseIndexer();
    indexer.index()
      .then(() => {
        console.log('Background indexing completed successfully');
        // Reload the search index
        search.loadIndex();
      })
      .catch(error => {
        console.error('Background indexing failed:', error);
      });
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// Utility functions
function formatBytes(bytes, decimals = 2) {
  if (bytes === 0) return '0 Bytes';
  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ['Bytes', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
}

async function downloadPregeneratedIndex() {
  console.log('No local embeddings index found.');
  console.log('Downloading pre-generated index from GitHub Gist...');
  console.log(`URL: ${PREGENERATED_INDEX_URL}`);
  
  try {
    const response = await fetch(PREGENERATED_INDEX_URL);
    
    if (!response.ok) {
      throw new Error(`Failed to download: ${response.status} ${response.statusText}`);
    }

    const data = await response.text();
    await fs.writeFile(INDEX_PATH, data, 'utf8');
    
    console.log('✓ Pre-generated index downloaded successfully!');
    console.log(`  Saved to: ${INDEX_PATH}`);
    
    // Parse to show stats
    const index = JSON.parse(data);
    console.log(`  Files indexed: ${index.metadata.totalFiles}`);
    console.log(`  Total embeddings: ${index.metadata.totalEmbeddings}`);
    console.log(`  Generated: ${new Date(index.metadata.generatedAt).toLocaleString()}`);
    
    return true;
  } catch (error) {
    console.error('✗ Failed to download pre-generated index:', error.message);
    console.error('  You can manually download it from:');
    console.error(`  ${PREGENERATED_INDEX_URL}`);
    console.error('  Or generate your own with: npm run index');
    return false;
  }
}

async function ensureIndexExists() {
  try {
    await fs.access(INDEX_PATH);
    return true; // Index exists
  } catch (error) {
    // Index doesn't exist, download it
    return await downloadPregeneratedIndex();
  }
}

// Start server
app.listen(PORT, async () => {
  console.log(`
╔════════════════════════════════════════════════════════════╗
║  Codebase Embeddings Search Server                        ║
╠════════════════════════════════════════════════════════════╣
║  Server running on: http://localhost:${PORT}                  ║
║  Open the demo:     http://localhost:${PORT}/index.html      ║
║                                                            ║
║  API Endpoints:                                            ║
║  - POST /api/search        : Search the codebase          ║
║  - GET  /api/metadata      : Get index metadata           ║
║  - GET  /api/index/status  : Check index status           ║
║  - POST /api/index/rebuild : Rebuild index                ║
╚════════════════════════════════════════════════════════════╝
  `);
  
  // Ensure index exists (download if needed)
  const indexReady = await ensureIndexExists();
  
  if (indexReady) {
    // Try to load index
    try {
      await search.loadIndex();
      const metadata = search.getMetadata();
      if (metadata) {
        console.log(`✓ Index loaded successfully!`);
        console.log(`  Files indexed: ${metadata.totalFiles}`);
        console.log(`  Total embeddings: ${metadata.totalEmbeddings}`);
        console.log(`  Generated: ${new Date(metadata.generatedAt).toLocaleString()}`);
        console.log();
        console.log('Ready to search! Open http://localhost:' + PORT + '/index.html');
      }
    } catch (error) {
      console.error('Failed to load index:', error.message);
      console.error('Try running: npm run index');
    }
  } else {
    console.log();
    console.log('⚠ Server started without an index.');
    console.log('  To use the demo, you need to either:');
    console.log('  1. Download the pre-generated index: npm run download-index');
    console.log('  2. Generate your own index: npm run index');
  }
});
