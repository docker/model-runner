# GPT‑OSS-safeguard

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/gpt-oss-safeguard-20b.png)

`gpt-oss-safeguard-120b` and `gpt-oss-safeguard-20b` are safety reasoning models built-upon gpt-oss. With these models, you can classify text content based on safety policies that you provide and perform a suite of foundational safety tasks. These models are intended for safety use cases. For other applications, we recommend using [gpt-oss models](https://huggingface.co/collections/openai/gpt-oss).

This model `gpt-oss-safeguard-20b` (21B parameters with 3.6B active parameters) fits into GPUs with 16GB of VRAM. Check out [`gpt-oss-safeguard-120b`](https://huggingface.co/openai/gpt-oss-safeguard-120b) (117B parameters with 5.1B active parameters) for the larger model.

Both models were trained on our [harmony response format](https://github.com/openai/harmony) and should only be used with the harmony format as it will not work correctly otherwise.

## Highlights

* **Trained to reason about safety** : Trained and tuned for safety reasoning to accommodate use cases like LLM input-output filtering, online content labeling and offline labeling for Trust and Safety use cases.
* **Bring your own policy:** Interprets your written policy, so it generalizes across products and use cases with minimal engineering.
* **Reasoned decisions, not just scores:** Gain complete access to the model’s reasoning process, facilitating easier debugging and increased trust in policy decisions. Keep in mind Raw CoT is meant for developers and safety practitioners. It’s not intended for exposure to general users or use cases outside of safety contexts.
* **Configurable reasoning effort:** Easily adjust the reasoning effort (low, medium, high) based on your specific use case and latency needs.
* **Permissive Apache 2.0 license:** Build freely without copyleft restrictions or patent risk—ideal for experimentation, customization, and commercial deployment.

## Inference examples

You can use gpt-oss-safeguard-120b and gpt-oss-safeguard-20b similar to gpt-oss-120b and gpt-oss-20b as described in our [respective cookbooks](https://cookbook.openai.com/topic/gpt-oss). We’ve also provided a detailed [prompting guide](https://cookbook.openai.com/articles/gpt-oss-safeguard-guide) that provides guidelines for how to craft your policy and use it with the models.

## Use this AI model with Docker Model Runner

Run the model:

```bash
docker model run ai/gpt-oss-safeguard
```

## Join the ROOST Model Community

gpt-oss-safeguard is a model partner of the [Robust Open Online Safety Tools (ROOST)](http://roost.tools/) Model Community. The ROOST Model Community (RMC) is a group of safety practitioners exploring open source AI models to protect online spaces. As an RMC model partner, OpenAI is committed to incorporating user feedback and jointly iterating on future releases in pursuit of open safety. Visit the [RMC GitHub repo](https://github.com/roostorg/open-models) to learn more about this partnership and how to get involved.

## Resources

* [Try gpt-oss-safeguard](https://huggingface.co/spaces/openai/gpt-oss-safeguard-20b)
* [OpenAI blog](https://openai.com/index/introducing-gpt-oss-safeguard/)