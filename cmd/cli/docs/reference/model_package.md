# docker model package

<!---MARKER_GEN_START-->
Package a GGUF file or Safetensors directory into a Docker model OCI artifact, with optional licenses. The package is sent to the model-runner, unless --push is specified.
When packaging a sharded GGUF model, --gguf should point to the first shard. All shard files should be siblings and should include the index in the file name (e.g. model-00001-of-00015.gguf).
When packaging a Safetensors model, --safetensors-dir should point to a directory containing .safetensors files and config files (*.json, merges.txt). All files will be auto-discovered and config files will be packaged into a tar archive.

### Options

| Name                | Type          | Default | Description                                                                            |
|:--------------------|:--------------|:--------|:---------------------------------------------------------------------------------------|
| `--chat-template`   | `string`      |         | absolute path to chat template file (must be Jinja format)                             |
| `--context-size`    | `uint64`      | `0`     | context size in tokens                                                                 |
| `--gguf`            | `string`      |         | absolute path to gguf file                                                             |
| `-l`, `--license`   | `stringArray` |         | absolute path to a license file                                                        |
| `--push`            | `bool`        |         | push to registry (if not set, the model is loaded into the Model Runner content store) |
| `--safetensors-dir` | `string`      |         | absolute path to directory containing safetensors files and config                     |


<!---MARKER_GEN_END-->

