# FunctionGemma
*GGUF version by Unsloth*

![logo](https://github.com/docker/model-cards/raw/refs/heads/main/logos/gemma-280x184-overview@2x.svg)

FunctionGemma is a lightweight 270M-parameter open model from Google, built on Gemma 3 and trained specifically for text-only function calling, designed to be fine-tuned into highly efficient, offline-capable specialized agents that can run on resource-constrained devices, as demonstrated by game logic and mobile action use cases.

## Intended uses

FunctionGemma is a lightweight, open model from Google, built as a foundation for creating your own specialized function calling models. FunctionGemma is not intended for use as a direct dialogue model, and is designed to be highly performant after further fine-tuning, as is typical of models this size. Built on the Gemma 3 270M model and with the same research and technology used to create the Gemini models, FunctionGemma has been trained specifically for function calling. The model has the same architecture as Gemma 3, but uses a different chat format. The model is well suited for text-only function calling. The uniquely small size makes it possible to deploy in environments with limited resources such as laptops, desktops or your own cloud infrastructure, democratizing access to state of the art AI models and helping foster innovation for everyone. Furthermore, akin to the base Gemma 270M, the model has been optimized to be extremely versatile, performant on a variety of hardware in single turn scenarios, but should be finetuned on single turn or multiturn task specific data to achieve best accuracy in specific domains. To demonstrate how specializing the 270M parameter model can achieve high performance on specific agentic workflows, we have highlighted two use cases in the Google AI Edge Gallery app.

Tiny Garden: A model fine-tuned to power a voice-controlled interactive game. It handles game logic to manage a virtual plot of land, decomposing commands like "Plant sunflowers in the top row" and "Water the flowers in plots 1 and 2" into app-specific functions (e.g., plant_seed, water_plots) and coordinate targets. This demonstrates the model's capacity to drive custom app mechanics without server connectivity.

Mobile Actions: To empower developers to build their own expert agents, we have published a dataset and fine-tuning recipe to demonstrate fine-tuning FunctionGemma. It translates user inputs (e.g., "Create a calendar event for lunch," "Turn on the flashlight") into function calls that trigger Android OS system tools. This interactive notebook demonstrates how to take the base FunctionGemma model and build a "Mobile Actions" fine tune from scratch for use in the Google AI Edge gallery app. This use case demonstrates the model's ability to act as an offline, private agent for personal device tasks.

## Inputs and outputs
Input:
- Text string, such as a question, a prompt, or a document to be summarized
- Total input context of 32K tokens

Output:
- Generated text in response to the input, such as an answer to a question, or a summary of a document
- Total output context up to 32K tokens per request, subtracting the request input tokens

## Use this AI model with Docker Model Runner

```bash
docker model run functiongemma
```

For more information on Docker Model Runner, [explore the documentation](https://docs.docker.com/desktop/features/model-runner/).

## Links
- [FunctionGemma model overview](https://ai.google.dev/gemma/docs/functiongemma)
- [Unsloth Dynamic 2.0 GGUF](https://docs.unsloth.ai/basics/unsloth-dynamic-2.0-ggufs)
