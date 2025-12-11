# Kimi K2

![logo](https://statics.moonshot.cn/kimi-blog/assets/logo-CvjirWOb.svg)

## Description
Kimi K2 Thinking is the latest, most capable version of open-source thinking model. Starting with Kimi K2, we built it as a thinking agent that reasons step-by-step while dynamically invoking tools. It sets a new state-of-the-art on Humanity's Last Exam (HLE), BrowseComp, and other benchmarks by dramatically scaling multi-step reasoning depth and maintaining stable tool-use across 200–300 sequential calls. At the same time, K2 Thinking is a native INT4 quantization model with 256k context window, achieving lossless reductions in inference latency and GPU memory usage.


## Key Features
- **Deep Thinking & Tool Orchestration:** End-to-end trained to interleave chain-of-thought reasoning with function calls, enabling autonomous research, coding, and writing workflows that last hundreds of steps without drift.
- **Native INT4 Quantization:** Quantization-Aware Training (QAT) is employed in post-training stage to achieve lossless 2x speed-up in low-latency mode.
- **Stable Long-Horizon Agency:** Maintains coherent goal-directed behavior across up to 200–300 consecutive tool invocations, surpassing prior models that degrade after 30–50 steps.

| **Field**                               | **Value**                |
|-----------------------------------------|--------------------------|
| Architecture                            | Mixture-of-Experts (MoE) |
| Total Parameters                        | 1T                       |
| Activated Parameters                    | 32B                      |
| Number of Layers (Dense layer included) | 61                       |
| Number of Dense Layers                  | 1                        |
| Attention Hidden Dimension              | 7168                     |
| MoE Hidden Dimension (per Expert)       | 2048                     |
| Number of Attention Heads               | 64                       |
| Number of Experts                       | 384                      |
| Selected Experts per Token              | 8                        |
| Number of Shared Experts                | 1                        |
| Vocabulary Size                         | 160K                     |
| Context Length                          | 256K                     |
| Attention Mechanism                     | MLA                      |
| Activation Function                     | SwiGLU                   |


## Use this AI model with Docker Model Runner

```bash
docker model run kimi-k2-vllm
```

## Benchmarks

### Reasoning Tasks
| Benchmark       | Setting   | K2 Thinking | GPT-5 (High) | Claude Sonnet 4.5 | K2 0905 (Thinking) | DeepSeek-V3.2 | Grok-4 |
|-----------------|-----------|-------------|--------------|-------------------|--------------------|---------------|--------|
| HLE             | no tools  | 23.9        | 26.3         | 19.8*             | 7.9                | 19.8          | 25.4   |
| HLE             | w/ tools  | 44.9        | 41.7*        | 32.0*             | 21.7               | 20.3*         | 41.0   |
| HLE             | heavy     | 51.0        | 42.0         | -                 | -                  | -             | 50.7   |
| AIME25          | no tools  | 94.5        | 94.6         | 87.0              | 51.0               | 89.3          | 91.7   |
| AIME25          | w/ python | 99.1        | 99.6         | 100.0             | 75.2               | 58.1*         | 98.8   |
| AIME25          | heavy     | 100.0       | 100.0        | -                 | -                  | -             | 100.0  |
| HMMT25          | no tools  | 89.4        | 93.3         | 74.6*             | 38.8               | 83.6          | 90.0   |
| HMMT25          | w/ python | 95.1        | 96.7         | 88.8*             | 70.4               | 49.5*         | 93.9   |
| HMMT25          | heavy     | 97.5        | 100.0        | -                 | -                  | -             | 96.7   |
| IMO-AnswerBench | no tools  | 78.6        | 76.0*        | 65.9*             | 45.8               | 76.0*         | 73.1   |
| GPQA            | no tools  | 84.5        | 85.7         | 83.4              | 74.2               | 79.9          | 87.5   |


### General Tasks

| Benchmark        | Setting  | K2 Thinking | GPT-5 (High) | Claude Sonnet 4.5 | K2 0905 (Thinking) | DeepSeek-V3.2 |
|------------------|----------|-------------|--------------|-------------------|--------------------|---------------|
| MMLU-Pro         | no tools | 84.6        | 87.1         | 87.5              | 81.9               | 85.0          |
| MMLU-Redux       | no tools | 94.4        | 95.3         | 95.6              | 92.7               | 93.7          |
| Longform Writing | no tools | 73.8        | 71.4         | 79.8              | 62.8               | 72.5          |
| HealthBench      | no tools | 58.0        | 67.2         | 44.2              | 43.8               | 46.9          |


### Agentic Search Tasks

| Benchmark        | Setting  | K2 Thinking | GPT-5 (High) | Claude Sonnet 4.5 | K2 0905 (Thinking) | DeepSeek-V3.2 |
|------------------|----------|-------------|--------------|-------------------|--------------------|---------------|
| BrowseComp       | w/ tools | 60.2        | 54.9         | 24.1              | 7.4                | 40.1          |
| BrowseComp-ZH    | w/ tools | 62.3        | 63.0*        | 42.4*             | 22.2               | 47.9          |
| Seal-0           | w/ tools | 56.3        | 51.4*        | 53.4*             | 25.2               | 38.5*         |
| FinSearchComp-T3 | w/ tools | 47.4        | 48.5*        | 44.0*             | 10.4               | 27.0*         |
| Frames           | w/ tools | 87.0        | 86.0*        | 85.0*             | 58.1               | 80.2*         |


### Coding Tasks

| Benchmark              | Setting                   | K2 Thinking | GPT-5 (High) | Claude Sonnet 4.5 | K2 0905 (Thinking) | DeepSeek-V3.2 |
|------------------------|---------------------------|-------------|--------------|-------------------|--------------------|---------------|
| SWE-bench Verified     | w/ tools                  | 71.3        | 74.9         | 77.2              | 69.2               | 67.8          |
| SWE-bench Multilingual | w/ tools                  | 61.1        | 55.3*        | 68.0              | 55.9               | 57.9          |
| Multi-SWE-bench        | w/ tools                  | 41.9        | 39.3*        | 44.3              | 33.5               | 30.6          |
| SciCode                | no tools                  | 44.8        | 42.9         | 44.7              | 30.7               | 37.7          |
| LiveCodeBenchV6        | no tools                  | 83.1        | 87.0*        | 64.0*             | 56.1*              | 74.1          |
| OJ-Bench (cpp)         | no tools                  | 48.7        | 56.2*        | 30.4*             | 25.5*              | 38.2*         |
| Terminal-Bench         | w/ simulated tools (JSON) | 47.1        | 43.8         | 51.0              | 44.5               | 37.7          |

## Links
- https://moonshotai.github.io/Kimi-K2/thinking.html
- https://huggingface.co/moonshotai/Kimi-K2-Thinking
