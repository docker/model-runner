const express = require('express');
const cors = require('cors');
const multer = require('multer');
const { PdfDataExtractor } = require('pdf-data-extractor');
const fs = require('fs').promises;
const path = require('path');

const app = express();
const PORT = process.env.PORT || 3000;

// Middleware
app.use(cors());
app.use(express.json());

// Configure multer for file upload
const upload = multer({ 
  dest: 'uploads/',
  limits: { fileSize: 10 * 1024 * 1024 } // 10MB limit
});

// Health check endpoint
app.get('/health', (req, res) => {
  res.json({ status: 'ok', message: 'PDF Data Extractor Demo Server' });
});

// Fetch available models from the API
app.post('/api/models', async (req, res) => {
  try {
    const { baseUrl } = req.body;
    
    if (!baseUrl) {
      return res.status(400).json({ error: 'Base URL is required' });
    }

    const response = await fetch(`${baseUrl}/models`);
    
    if (!response.ok) {
      return res.status(response.status).json({ 
        error: `Failed to fetch models: ${response.statusText}` 
      });
    }

    const data = await response.json();
    res.json(data);
  } catch (error) {
    console.error('Error fetching models:', error);
    res.status(500).json({ 
      error: 'Failed to fetch models',
      message: error.message 
    });
  }
});

// Extract data from PDF
app.post('/api/extract', upload.single('pdf'), async (req, res) => {
  let pdfPath = null;
  
  try {
    // Validate request
    if (!req.file) {
      return res.status(400).json({ error: 'No PDF file provided' });
    }

    const { schema, baseUrl, model, apiKey, temperature, maxTokens } = req.body;

    if (!schema) {
      return res.status(400).json({ error: 'No schema provided' });
    }

    if (!baseUrl) {
      return res.status(400).json({ error: 'No base URL provided' });
    }

    if (!model) {
      return res.status(400).json({ error: 'No model provided' });
    }

    // Parse schema
    let parsedSchema;
    try {
      parsedSchema = JSON.parse(schema);
    } catch (error) {
      return res.status(400).json({ 
        error: 'Invalid JSON schema',
        message: error.message 
      });
    }

    // Initialize extractor with provided configuration
    const extractor = new PdfDataExtractor({
      openaiApiKey: apiKey || 'not-required-for-local-models',
      model: model,
      baseUrl: baseUrl
    });

    // Extract data from uploaded PDF
    pdfPath = req.file.path;
    
    const extractOptions = {
      pdfPath: pdfPath,
      schema: parsedSchema
    };

    // Add optional parameters if provided
    if (temperature !== undefined && temperature !== '') {
      extractOptions.temperature = parseFloat(temperature);
    }
    if (maxTokens !== undefined && maxTokens !== '') {
      extractOptions.maxTokens = parseInt(maxTokens);
    }

    console.log(`Extracting data from PDF using model: ${model}`);
    const result = await extractor.extract(extractOptions);

    // Clean up uploaded file
    await fs.unlink(pdfPath);
    pdfPath = null;

    // Return results
    res.json({
      success: true,
      data: result.data,
      tokensUsed: result.tokensUsed,
      model: result.model
    });

  } catch (error) {
    console.error('Error extracting data:', error);
    
    // Clean up file if it exists
    if (pdfPath) {
      try {
        await fs.unlink(pdfPath);
      } catch (cleanupError) {
        console.error('Error cleaning up file:', cleanupError);
      }
    }

    res.status(500).json({ 
      success: false,
      error: 'Failed to extract data from PDF',
      message: error.message 
    });
  }
});

// Start server
app.listen(PORT, () => {
  console.log(`PDF Data Extractor Demo Server running on http://localhost:${PORT}`);
  console.log(`Upload endpoint: http://localhost:${PORT}/api/extract`);
});

// Create uploads directory if it doesn't exist
const uploadsDir = path.join(__dirname, 'uploads');
fs.mkdir(uploadsDir, { recursive: true }).catch(console.error);
