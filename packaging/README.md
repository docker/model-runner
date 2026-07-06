# Packaging dmr

The standalone `dmr` binary (see [`cmd/dmr`](../cmd/dmr)) is distributed via:

- **Homebrew** — `brew install docker/tap/dmr` (macOS, arm64)
- **WinGet** — `winget install Docker.dmr` (Windows, amd64)
- **Direct download** — cross-compiled archives attached to each
  [GitHub release](https://github.com/docker/model-runner/releases) tagged
  `dmr-vX.Y.Z` (macOS arm64, Linux amd64/arm64, Windows amd64)

There are no static package manifests checked into this repository: the
[`release-dmr.yml`](../.github/workflows/release-dmr.yml) workflow builds the
binaries, creates the GitHub release, generates the Homebrew formula and
opens a PR against `docker/homebrew-tap`, and submits the WinGet manifest to
`microsoft/winget-pkgs` via `wingetcreate`, all from the version being
released. This mirrors the packaging approach used by
[`docker/sandboxes`](https://github.com/docker/sandboxes)'s
`publish-brew.yml`/`publish-winget.yml`.

## Cutting a release

```
git tag dmr-v0.1.0
git push origin dmr-v0.1.0
```

This triggers `release-dmr.yml`, which requires the following repository
secrets:

- `HOMEBREW_TAP_TOKEN` — token with push/PR access to `docker/homebrew-tap`
- `WINGET_GH_KEY` — token used by `wingetcreate` to submit to
  `microsoft/winget-pkgs`

## Local cross-compilation

```
make build-dmr-cross
```

builds every published target into `dist/dmr/<os>-<arch>/dmr`. dmr has no
cgo dependencies, so all targets cross-compile cleanly from any host.

Linux `.deb`/`.rpm` packages continue to be produced by the existing
`docker-model-plugin` packaging pipeline (see `.github/workflows/release.yml`)
and are unaffected by this workflow.
