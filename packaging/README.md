# Packaging dmr

The standalone `dmr` binary (see [`cmd/dmr`](../cmd/dmr)) is distributed via:

- **Homebrew** — `brew install --cask docker/tap/dmr` (macOS, arm64;
  signed + notarized)
- **WinGet** — `winget install Docker.dmr` (Windows, amd64)
- **Direct download** — cross-compiled archives attached to each
  [GitHub release](https://github.com/docker/model-runner/releases) tagged
  `dmr-vX.Y.Z` (macOS arm64, Linux amd64/arm64, Windows amd64)

There are no static package manifests checked into this repository. The
release is a **two-repo, two-step** flow because macOS/Windows code signing
requires Docker's private signing credentials, which cannot live in this
public repository:

1. [`release-dmr.yml`](../.github/workflows/release-dmr.yml) (here) builds all
   targets, creates the GitHub release with the (unsigned) archives, submits
   the WinGet manifest, and auto-triggers step 2.
2. `release-dmr.yml` in the internal
   [`docker/inference-engine-llama.cpp`](https://github.com/docker/inference-engine-llama.cpp)
   repo downloads the macOS/Windows archives from this release, signs and
   notarizes them (via `docker/desktop-action-private`), re-uploads the signed
   versions to this same release, and opens the Homebrew **cask** PR against
   `docker/homebrew-tap`.

This mirrors how [`docker/sandboxes`](https://github.com/docker/sandboxes)
signs and publishes `sbx` as a cask.

## Cutting a release

```
git tag dmr-v0.1.0
git push origin dmr-v0.1.0
```

This triggers `release-dmr.yml`, which requires (cross-repo automation uses a
GitHub App, never a PAT):

- `DMR_TRIGGER_APP_ID` (variable) + `DMR_TRIGGER_APP_PRIVATE_KEY` (secret) — a
  dedicated GitHub App with only `Actions: write` on
  `docker/inference-engine-llama.cpp`, used to dispatch the signing workflow
  there (step 2).
- `WINGET_GH_KEY` (secret) — token used by `wingetcreate` to submit to
  `microsoft/winget-pkgs`.

Step 2 runs in the internal `docker/inference-engine-llama.cpp` repo, which
holds its own signing and release credentials.

## Local cross-compilation

```
make build-dmr-cross
```

builds every published target into `dist/dmr/<os>-<arch>/dmr`. dmr has no
cgo dependencies, so all targets cross-compile cleanly from any host.

Linux `.deb`/`.rpm` packages continue to be produced by the existing
`docker-model-plugin` packaging pipeline (see `.github/workflows/release.yml`)
and are unaffected by this workflow.
