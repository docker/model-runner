# Agents

## Before committing

Run all CI validations locally before committing changes:

```
make validate-all
```

This runs the same checks as CI:

1. **go mod tidy** — ensures `go.mod`/`go.sum` are clean
2. **Lint** — runs `golangci-lint` (see [`.golangci.yml`](./.golangci.yml) for configuration)
3. **Tests** — runs all unit tests with race detection (`go test -race ./...`)
4. **Shellcheck** — validates all shell scripts

If any step fails, fix the issue and re-run before committing.

### Prerequisites

- **Go 1.25.6+**
- **golangci-lint v2.7.2+** — [Install instructions](https://golangci-lint.run/welcome/install/)
- **shellcheck** — `brew install shellcheck` (macOS) or `apt-get install shellcheck` (Linux)

## Project documentation

- [README.md](./README.md) — project overview, building from source, API examples, and Makefile usage
- [METRICS.md](./METRICS.md) — aggregated metrics endpoint documentation
- [Model CLI README](./cmd/cli/README.md) — CLI plugin (`docker model`) documentation
- [Helm chart](./charts/docker-model-runner/README.md) — Kubernetes deployment guide
- [Model Specification](https://github.com/docker/model-spec/blob/main/spec.md) — model packaging specification
- [Docker Docs](https://docs.docker.com/ai/model-runner/get-started/) — official Docker Model Runner documentation
