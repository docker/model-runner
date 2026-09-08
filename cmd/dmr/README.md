# dmr

`dmr` is the standalone Docker Model Runner: a single binary that bundles
both the inference daemon and the full model management CLI, with **no
dependency on Docker Desktop or a running Docker Engine**.

```
dmr serve              # start the daemon (foreground)
dmr pull ai/gemma3     # pull a model
dmr run ai/gemma3 "Hi" # run it
dmr ls                 # list local models
dmr ps                 # list running models
dmr rm ai/gemma3       # remove a local model
```

## How it fits together

- `dmr serve` runs [`pkg/server`](../../pkg/server) directly, in-process —
  the same daemon used by the `docker-model-runner` container and by Docker
  Desktop's bundled runner. It listens on TCP port `12434` by default (or a
  Unix socket via `--socket`); see `dmr serve --help`.
- Every other `dmr` subcommand is the same command tree as the `docker
  model` CLI plugin ([`cmd/cli/commands`](../cli/commands)), so it has full
  feature parity (`run`, `pull`, `push`, `tag`, `inspect`, `logs`, `bench`,
  `configure`, ...). See [`cmd/cli/README.md`](../cli/README.md) for full
  command documentation.
- `dmr` always talks to its daemon over `MODEL_RUNNER_HOST` (default
  `http://localhost:12434`), which forces the "manual host" code path in
  [`cmd/cli/desktop/context.go`](../cli/desktop/context.go). This is what
  makes `dmr` engine-independent: unlike `docker model`, it never probes a
  Docker Engine connection or checks for Docker Desktop.
- Docker-Engine-only commands that manage a `docker-model-runner`
  *container* (`install-runner`, `start-runner`, `stop-runner`,
  `restart-runner`, `reinstall-runner`, `uninstall-runner`) are hidden from
  `dmr`, since `dmr serve` replaces them — there's no container to manage.

## Building

```
make build-dmr         # native build, output ./dmr (or dmr.exe on Windows)
make build-dmr-cross   # cross-compile every published target into dist/dmr/<os>-<arch>/
```

`dmr` has no cgo dependencies, so it cross-compiles cleanly for macOS
(arm64), Linux (amd64/arm64), and Windows (amd64) from any host. See
[`../../packaging/README.md`](../../packaging/README.md) for how these
builds are released via Homebrew (`brew install docker/tap/dmr`) and WinGet
(`winget install Docker.dmr`).
