package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/docker/model-runner/pkg/server"
)

// exitFunc is used for Fatal-like exits; overridden in tests.
var exitFunc = func(code int) { os.Exit(code) }

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := server.Run(ctx, server.Config{Version: Version, ExitFunc: exitFunc}); err != nil {
		fmt.Fprintf(os.Stderr, "model-runner: %v\n", err)
		exitFunc(1)
	}
}
