# DeepSeek-V3.2: Efficient Reasoning & Agentic AI

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/deepseek-280x184-overview@2x.svg)

We introduce DeepSeek-V3.2, a model that harmonizes high computational efficiency with superior reasoning and agent performance. Our approach is built upon three key technical breakthroughs:
1. **DeepSeek Sparse Attention (DSA):** We introduce DSA, an efficient attention mechanism that substantially reduces computational complexity while preserving model performance, specifically optimized for long-context scenarios.
2. **Scalable Reinforcement Learning Framework:** By implementing a robust RL protocol and scaling post-training compute, DeepSeek-V3.2 performs comparably to GPT-5. Notably, our high-compute variant, DeepSeek-V3.2-Speciale, surpasses GPT-5 and exhibits reasoning proficiency on par with Gemini-3.0-Pro.
3. **Large-Scale Agentic Task Synthesis Pipeline:** To integrate reasoning into tool-use scenarios, we developed a novel synthesis pipeline that systematically generates training data at scale. This facilitates scalable agentic post-training, improving compliance and generalization in complex interactive environments.
   
*Achievement*: 🥇 Gold-medal performance in the 2025 International Mathematical Olympiad (IMO) and International Olympiad in Informatics (IOI).

![benchmark](https://huggingface.co/deepseek-ai/DeepSeek-V3.2/resolve/main/assets/benchmark.png)

## Use this AI model with Docker Model Runner

```bash
docker model run deepseek-v3.2-vllm
```

For more information on Docker Model Runner, [explore the documentation](https://docs.docker.com/desktop/features/model-runner/).

## Usage tips

- Recommended values temperature 1 and top_p 0.95

## Links
- https://huggingface.co/deepseek-ai/DeepSeek-V3.2
- https://github.com/deepseek-ai/DeepSeek-V3.2-Exp
