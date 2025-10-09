# PDF Data Extractor Demo

This demo application allows you to extract structured data from PDF documents using JSON schemas and AI models.

## Features

- 📄 Upload and process PDF files
- 📋 Define custom JSON schemas for data extraction
- 🎯 Pre-built schema examples (Invoice, Receipt, Form)
- 📊 View extracted data with token usage statistics
- ⚙️ Configurable temperature and model selection

## Prerequisites

Before running this demo, you need:

1. **Node.js** (version 18 or higher)
2. **Docker Model Runner**
3. **A suitable AI model** for text extraction

## Setup Instructions

### 1. Enable Docker Model Runner

**Using Docker Desktop:**
- Open Docker Desktop settings
- Go to the **AI** tab
- Select **Enable Docker Model Runner**
- Enable **host-side TCP support** on port `12434` (default)

For detailed instructions, see the [Docker Model Runner documentation](https://docs.docker.com/ai/model-runner/get-started/#enable-docker-model-runner).

**Using Standalone Docker Engine:**
TCP support is enabled by default on port `12434`.

#### 2. Pull a Suitable Model

You'll need a model capable of understanding and extracting text. Recommended models:

```bash
# Pull a general-purpose model
docker model pull ai/gemma3
```

To see available models, visit [Docker Hub - AI Models](https://hub.docker.com/r/ai).

## Installation

1. **Navigate to the demo directory:**
   ```bash
   cd demos/extractor
   ```

2. **Install dependencies:**
   ```bash
   npm install
   ```

3. **Start the server:**
   ```bash
   npm start
   ```

   The server will start on `http://localhost:3000`

4. **Open the demo:**
   Open `demo.html` in your web browser (you can simply double-click the file or serve it with a local server)

## Usage Guide

### Basic Workflow

1. **Configure API Settings**
   - **Base API URL**: Set to `http://127.0.0.1:12434/engines/v1` for Docker Model Runner
   - **Model**: Select from available models

2. **Define Your Schema**
   - Use the provided examples (Invoice, Receipt, Form) or create your own
   - The schema defines what data to extract from the PDF
   - Use standard JSON Schema format with `type`, `properties`, etc.

3. **Upload a PDF**
   - Click "Choose File" and select your PDF document
   - Supported: Any text-based PDF (not scanned images without OCR)
   - You can use sample PDFs [invoice.pdf](invoice.pdf)

4. **Extract Data**
   - Click "Extract Data" button
   - Wait for processing (may take 10-30 seconds depending on PDF size and model)
   - View extracted data in the result section
