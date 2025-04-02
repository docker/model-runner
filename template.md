# {model name}

![logo](logo)

Description

## Available Model Variants

| Model Variant               | Parameters | Quantization   | Context Window | VRAM      | Size   |
|---------------------------- |----------- |--------------- |--------------- |---------- |------- |
| {name}:{params]_{quant]     | {param}    | {quant}        | {token}        | {size}GB¹ | {size} | 

¹: VRAM estimates based on model characteristics.

## Characteristics

| Attribute             | Details        |
|---------------------- |----------------|
| **Provider**          | {creator}      |
| **Architecture**      | {arch}         |
| **Cutoff Date**       | {date}         |
| **Languages**         | {language_list}|
| **Tool Calling**      | {yes/no}       |
| **Input Modalities**  | {input_list}   |
| **Output Modalities** | {output_list}  |
| **License**           | {license}      |

## Intended Uses

{small description}

- **{case name }**: {description}
- **{case name }**: {description}
- **{case name }**: {description}

## Considerations

- {recommendation1}
- {recommendationn}
{notes}

## How to Run This AI Model

You can pull the model using:
```
docker model pull {model_name}
```

To run the model:
```
docker model run {model_name}
```

## Benchmark Performance

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
