# {model name}

## Description
{small description}

## Characteristics

| Attribute             | Details        |
|---------------------- |----------------|
| **Provider**          | {creator}      |
| **Architecture**      | {arch}         |
| **Cutoff date**       | {date}         |
| **Languages**         | {language_list}|
| **Tool calling**      | {yes/no}       |
| **Input modalities**  | {input_list}   |
| **Output modalities** | {output_list}  |
| **License**           | {license}      |

## Available model variants

| Model Variant               | Parameters | Quantization   | Context Window | VRAM      | Size   |
|---------------------------- |----------- |--------------- |--------------- |---------- |------- |
| {name}:{params]_{quant]     | {param}    | {quant}        | {token}        | {size}GB¹ | {size} | 

¹: VRAM estimates based on model characteristics.

## Use this AI model with Docker Model Runner

```bash
docker model run {model_name}
```

## Considerations

- {recommendation1}
- {recommendationn}
{notes}

## Benchmark performance

| Category    | Metric                      | {model_name} |
|-------------|-----------------------------|------------- |
| **{name}**  |                             |              |
|             | {metric}                    | {value}      |
|             | {metric}                    | {value}      |
|             | {metric}                    | {value}      |
| **{name}**  |                             |              |
|             | {metric}                    | {value}      |
|             | {metric}                    | {value}      |

## Links
- {reference_link}
