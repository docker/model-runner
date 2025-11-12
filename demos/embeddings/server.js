const express = require('express');
const cors = require('cors');
const path = require('path');
const SemanticSearch = require('./search');
const CodebaseIndexer = require('./indexer');

const app = express();
const PORT = process.env.PORT || 3000;

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

// Utility function
function formatBytes(bytes, decimals = 2) {
  if (bytes === 0) return '0 Bytes';
  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ['Bytes', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
}

// Start server
app.listen(PORT, () => {
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
  
  // Try to load index on startup
  search.loadIndex()
    .then(() => {
      const metadata = search.getMetadata();
      if (metadata) {
        console.log(`Loaded index with ${metadata.totalEmbeddings} embeddings from ${metadata.totalFiles} files`);
      }
    })
    .catch(() => {
      console.log('No index found. Run "npm run index" to create one.');
    });
});
