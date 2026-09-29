// Command dmr is the standalone Docker Model Runner: a single binary that
// bundles both the inference daemon ("dmr serve") and the full model
// management CLI (run, ls, pull, rm, ps, ...) used to talk to it.
//
// dmr has no dependency on Docker Desktop or a running Docker Engine: the
// daemon is a plain HTTP process, and the CLI always talks to it directly
// over MODEL_RUNNER_HOST (see root.go). This makes dmr suitable for use on
// any macOS or Linux host, with or without Docker installed.
package main

import (
	"fmt"
	"os"
)

// Version is set at build time via -ldflags "-X main.Version=...".
var Version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
