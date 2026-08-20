# docker model config

<!---MARKER_GEN_START-->
Read and write model-runner config file values.

The config file uses an INI format with sections and key=value pairs:

    [section]
        key = value
    [section "subsection"]
        key = value

Keys are specified in dot notation: section.key or section.subsection.key.

The default file is $XDG_CONFIG_HOME/model-runner/config, falling back to
~/.config/model-runner/config when XDG_CONFIG_HOME is not set.

Examples:
    model-cli config set user.name "Alice"
    model-cli config get user.name
    model-cli config list
    model-cli config unset user.name
    model-cli config edit

### Subcommands

| Name                             | Description                         |
|:---------------------------------|:------------------------------------|
| [`edit`](model_config_edit.md)   | Open the config file in your editor |
| [`get`](model_config_get.md)     | Get the value of a config key       |
| [`list`](model_config_list.md)   | List all config key/value pairs     |
| [`set`](model_config_set.md)     | Set a config key to a value         |
| [`unset`](model_config_unset.md) | Remove a config key                 |



<!---MARKER_GEN_END-->

