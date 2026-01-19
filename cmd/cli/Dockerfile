# syntax=docker/dockerfile:1

ARG GO_VERSION=1.25
ARG ALPINE_VERSION=3.23

ARG DOCS_FORMATS="md,yaml"

FROM --platform=${BUILDPLATFORM} golang:${GO_VERSION}-alpine${ALPINE_VERSION} AS base
RUN apk add --no-cache rsync git
ENV CGO_ENABLED=0
WORKDIR /src

FROM base AS docs-gen
RUN --mount=target=. \
    --mount=target=/root/.cache,type=cache \
    go build -C cmd/cli -o /out/docsgen ./docs/generate.go

FROM base AS docs-build
COPY --from=docs-gen /out/docsgen /usr/bin
ENV DOCKER_CLI_PLUGIN_ORIGINAL_CLI_COMMAND="model"
ARG DOCS_FORMATS
RUN --mount=target=/context \
    --mount=target=.,type=tmpfs <<EOT
  set -e
  rsync -a --exclude='models-store' /context/. .
  docsgen --formats "$DOCS_FORMATS" --source "cmd/cli/docs/reference"
  mkdir /out
  cp -r cmd/cli/docs/reference/* /out/
EOT

FROM scratch AS docs-update
COPY --from=docs-build /out /

FROM docs-build AS docs-validate
RUN --mount=target=/context \
    --mount=target=.,type=tmpfs <<EOT
  set -e
  rsync -a --exclude='models-store' /context/. .
  git add -A
  rm -rf cmd/cli/docs/reference/*
  cp -rf /out/* ./cmd/cli/docs/reference/
  if [ -n "$(git status --porcelain -- cmd/cli/docs/reference)" ]; then
    echo >&2 'ERROR: Docs result differs. Please update with "make docs"'
    git status --porcelain -- cmd/cli/docs/reference
    exit 1
  fi
EOT

FROM base AS test
RUN apk add --no-cache make gcc musl-dev
WORKDIR /src
RUN --mount=target=. \
  --mount=target=/root/.cache,type=cache \
  CGO_ENABLED=1 make -C cmd/cli unit-tests
