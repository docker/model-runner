# 📦 Multi-Model Repository

Find descriptions and details about various AI models, including their capabilities, use cases, and specifications.

---

## 🚀 Models Overview

### DeepSeek R1
![DeepSeek R1 Logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/deepseek-120x-hub@2x.svg)

📌 **Description:**  
Distilled LLaMA by DeepSeek, fast and optimized for real-world tasks.

📂 **Model File:** [`ai/deepseek-r1-distill-llama.md`](ai/deepseek-r1-distill-llama.md)

**URLs:**
- https://huggingface.co/deepseek-ai/DeepSeek-R1-Distill-Llama-8B
- https://huggingface.co/deepseek-ai/DeepSeek-R1-Distill-Llama-70B

---

### Gemma 3
![Gemma Logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/gemma-120x-hub@2x.svg)

📌 **Description:**  
Google's latest Gemma, small yet strong for chat and generation

📂 **Model File:** [`ai/gemma3.md`](ai/gemma3.md)

**URLs:**
- https://huggingface.co/google/gemma-3-4b-it

---

### Llama 3.1
![Meta Logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/meta-120x-hub@2x.svg)

📌 **Description:**  
Meta's LLaMA 3.1: Chat-focused, benchmark-strong, multilingual-ready.

📂 **Model File:** [`ai/llama3.1.md`](ai/llama3.1.md)

**URLs:**
- https://huggingface.co/meta-llama/Llama-3.1-8B-Instruct

---

### Llama 3.2
![Meta Logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/meta-120x-hub@2x.svg)

📌 **Description:**  
Solid LLaMA 3 update, reliable for coding, chat, and Q&A tasks.

📂 **Model File:** [`ai/llama3.2.md`](ai/llama3.2.md)

**URL:**
- https://huggingface.co/meta-llama/Llama-3.2-3B-Instruct
- https://huggingface.co/meta-llama/Llama-3.2-1B-Instruct

---
### Llama 3.3

![Meta Logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/meta-120x-hub@2x.svg)

📌 **Description:**  
Newest LLaMA 3 release with improved reasoning and generation quality.

📂 **Model File:** [`ai/llama3.3.md`](ai/llama3.3.md)

---

### Mistral 7b
![Mistral Logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/mistral-120x-hub@2x.svg)

📌 **Description:**  
A fast and powerful 7B parameter model excelling in reasoning, code, and math.

📂 **Model File:** [`ai/mistral.md`](ai/mistral.md)

**URLs:**
- https://huggingface.co/mistralai/Mistral-7B-Instruct-v0.3

---
### Mistral Nemo
![Mistral Logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/mistral-120x-hub@2x.svg)

📌 **Description:**  
Mistral-Nemo-Instruct-2407 is an instruct fine-tuned large language model developed by Mistral AI and NVIDIA.

📂 **Model File:** [`ai/mistral-nemo.md`](ai/mistral-nemo.md)

**URLs:**
- https://huggingface.co/mistralai/Mistral-Nemo-Instruct-2407

---
### mxbai-embed-large
![Mixedbread Logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/mixedbread-120x-hub@2x.svg)

📌 **Description:**  
A state-of-the-art English language embedding model developed by Mixedbread AI.

📂 **Model File:** [`ai/mxbai-embed-large.md`](ai/mxbai-embed-large.md)

**URLs:**
- https://huggingface.co/mixedbread-ai/mxbai-embed-large-v1

---

### Phi-4
![Microsoft Logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/phi-120x-hub@2x.svg)

📌 **Description:**  
Microsoft's compact model, surprisingly capable at reasoning and code.

📂 **Model File:** [`ai/phi4.md`](ai/phi4.md)

**URLs:**
- https://huggingface.co/microsoft/phi-4

---

### Qwen 2.5
![Qwen Logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/qwen-120x-hub@2x.svg)

📌 **Description:**  
Versatile Qwen update with better language skills and wider support.

📂 **Model File:** [`ai/qwen2.5.md`](ai/qwen2.5.md)

**URLs:**
- https://huggingface.co/Qwen/Qwen2.5-7B-Instruct

---

### QwQ
![Qwen Logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/qwen-120x-hub@2x.svg)

📌 **Description:**  
Experimental Qwen variant—lean, fast, and a bit mysterious.

📂 **Model File:** [`ai/qwq.md`](ai/qwq.md)

**URLs:**
- https://huggingface.co/Qwen/QwQ-32B

---

### SmolLM 2
![Huggingface Logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/hugginfface-120x-hub@2x.svg)

📌 **Description:**  
A compact language model, designed to run efficiently on-device while performing a wide range of language tasks 

📂 **Model File:** [`ai/smollm2.md`](ai/smollm2.md)

**URLs:**
- https://huggingface.co/HuggingFaceTB/SmolLM2-360M-Instruct
- https://huggingface.co/HuggingFaceTB/SmolLM2-135M-Instruct

---

## 🔧 CLI Usage

The model-cards-cli tool provides commands to inspect and update model information:

### Inspect Command
```bash
# Basic inspection
make inspect REPOSITORY=ai/smollm2

# Inspect specific tag
make inspect REPOSITORY=ai/smollm2 TAG=360M-Q4_K_M

# Show all metadata
make inspect REPOSITORY=ai/smollm2 OPTIONS="--all"
```

### Update Command
```bash
# Update all models
make run

# Update specific model
make run-single MODEL=smollm2.md
```

### Upload Overview Command
```bash
# Upload a single overview to Docker Hub
make -C tools/model-cards-cli upload-overview FILE=ai/llama3.1.md REPO=ai/llama3 USERNAME=your_username TOKEN=your_pat_here

# Upload all overviews in the ai/ folder to their corresponding repositories
./tools/upload-all-overviews.sh your_username your_pat_here
```

### Available Options

#### Inspect Command Options
- `REPOSITORY`: (Required) The repository to inspect (e.g., `ai/smollm2`)
- `TAG`: (Optional) Specific tag to inspect (e.g., `360M-Q4_K_M`)
- `OPTIONS`: (Optional) Additional options:
  - `--all`: Show all metadata fields
  - `--log-level`: Set log level (debug, info, warn, error)

#### Update Command Options
- `MODEL`: (Required for run-single) Specific model file to update (e.g., `ai/smollm2.md`)
- `--log-level`: Set log level (debug, info, warn, error)

#### Upload Overview Options
- `FILE`: (Required) Path to the markdown file containing the overview content
- `REPO`: (Required) Repository to upload the overview to (format: namespace/repository)
- `USERNAME`: (Required) Docker Hub username
- `TOKEN`: (Required) Personal Access Token (PAT)
