# docker model config get

<!---MARKER_GEN_START-->
Get the value of a config key.

Prints the value of the given key to stdout. If the key appears multiple times
(multi-valued), the last value is printed. Use --all to print all values.

Exit status is 1 if the key is not found (unless --default is given).

### Options

| Name            | Type     | Default | Description                               |
|:----------------|:---------|:--------|:------------------------------------------|
| `--all`         | `bool`   |         | print all values for multi-valued keys    |
| `--default`     | `string` |         | value to emit if the key is not set       |
| `-f`, `--file`  | `string` |         | use a specific config file                |
| `--global`      | `bool`   |         | use the global (user-level) config file   |
| `--show-origin` | `bool`   |         | show the origin (file path) of each value |
| `--system`      | `bool`   |         | use the system-wide config file           |


<!---MARKER_GEN_END-->

