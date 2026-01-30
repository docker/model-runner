# docker model

<!---MARKER_GEN_START-->
Docker Model Runner

### Subcommands

| Name                                            | Description                                                                                                |
|:------------------------------------------------|:-----------------------------------------------------------------------------------------------------------|
| [`bench`](model_bench.md)                       | Benchmark a model's performance at different concurrency levels                                            |
| [`df`](model_df.md)                             | Show Docker Model Runner disk usage                                                                        |
| [`inspect`](model_inspect.md)                   | Display detailed information on one model                                                                  |
| [`install-runner`](model_install-runner.md)     | Install Docker Model Runner (Docker Engine only)                                                           |
| [`list`](model_list.md)                         | List the models pulled to your local environment                                                           |
| [`logs`](model_logs.md)                         | Fetch the Docker Model Runner logs                                                                         |
| [`package`](model_package.md)                   | Package a GGUF file, Safetensors directory, DDUF file, or existing model into a Docker model OCI artifact. |
| [`ps`](model_ps.md)                             | List running models                                                                                        |
| [`pull`](model_pull.md)                         | Pull a model from Docker Hub or HuggingFace to your local environment                                      |
| [`purge`](model_purge.md)                       | Remove all models                                                                                          |
| [`push`](model_push.md)                         | Push a model to Docker Hub                                                                                 |
| [`reinstall-runner`](model_reinstall-runner.md) | Reinstall Docker Model Runner (Docker Engine only)                                                         |
| [`requests`](model_requests.md)                 | Fetch requests+responses from Docker Model Runner                                                          |
| [`restart-runner`](model_restart-runner.md)     | Restart Docker Model Runner (Docker Engine only)                                                           |
| [`rm`](model_rm.md)                             | Remove local models downloaded from Docker Hub                                                             |
| [`run`](model_run.md)                           | Run a model and interact with it using a submitted prompt or chat mode                                     |
| [`search`](model_search.md)                     | Search for models on Docker Hub and HuggingFace                                                            |
| [`show`](model_show.md)                         | Show information for a model                                                                               |
| [`start-runner`](model_start-runner.md)         | Start Docker Model Runner (Docker Engine only)                                                             |
| [`status`](model_status.md)                     | Check if the Docker Model Runner is running                                                                |
| [`stop-runner`](model_stop-runner.md)           | Stop Docker Model Runner (Docker Engine only)                                                              |
| [`tag`](model_tag.md)                           | Tag a model                                                                                                |
| [`uninstall-runner`](model_uninstall-runner.md) | Uninstall Docker Model Runner (Docker Engine only)                                                         |
| [`unload`](model_unload.md)                     | Unload running models                                                                                      |
| [`version`](model_version.md)                   | Show the Docker Model Runner version                                                                       |


### Options

| Name                | Type     | Default                             | Description                                                                                                                           |
|:--------------------|:---------|:------------------------------------|:--------------------------------------------------------------------------------------------------------------------------------------|
| `--config`          | `string` | `/Users/yuxuanche/.docker`          | Location of client config files                                                                                                       |
| `-c`, `--context`   | `string` |                                     | Name of the context to use to connect to the daemon (overrides DOCKER_HOST env var and default context set with "docker context use") |
| `-D`, `--debug`     | `bool`   |                                     | Enable debug mode                                                                                                                     |
| `-H`, `--host`      | `string` |                                     | Daemon socket to connect to                                                                                                           |
| `-l`, `--log-level` | `string` | `info`                              | Set the logging level ("debug", "info", "warn", "error", "fatal")                                                                     |
| `--tls`             | `bool`   |                                     | Use TLS; implied by --tlsverify                                                                                                       |
| `--tlscacert`       | `string` | `/Users/yuxuanche/.docker/ca.pem`   | Trust certs signed only by this CA                                                                                                    |
| `--tlscert`         | `string` | `/Users/yuxuanche/.docker/cert.pem` | Path to TLS certificate file                                                                                                          |
| `--tlskey`          | `string` | `/Users/yuxuanche/.docker/key.pem`  | Path to TLS key file                                                                                                                  |
| `--tlsverify`       | `bool`   |                                     | Use TLS and verify the remote                                                                                                         |


<!---MARKER_GEN_END-->

## Description

Use Docker Model Runner to run and interact with AI models directly from the command line.
For more information, see the [documentation](https://docs.docker.com/ai/model-runner/)
