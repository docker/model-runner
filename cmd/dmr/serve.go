package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/docker/model-runner/pkg/server"
	"github.com/spf13/cobra"
)

// defaultPort is the TCP port the daemon listens on when neither --port,
// --socket, MODEL_RUNNER_PORT, nor MODEL_RUNNER_SOCK is set. It matches
// defaultHost above so that the bundled CLI can find a freshly started
// daemon with zero configuration.
const defaultPort = "12434"

// serveOptions holds the flags exposed by "dmr serve". Each maps onto an
// environment variable consumed by pkg/envconfig; flags take precedence
// over any pre-existing environment variable.
type serveOptions struct {
	port       string
	socket     string
	modelsPath string
}

func newServeCmd() *cobra.Command {
	var opts serveOptions

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Docker Model Runner daemon",
		Long: `Start the Docker Model Runner daemon.

The daemon manages the lifecycle of inference backends (llama.cpp, vLLM,
SGLang, MLX, diffusers) and exposes an OpenAI-compatible HTTP API for
listing, pulling, and running models. It requires no Docker Desktop or
Docker Engine installation.

By default it listens on TCP port 12434. Use --socket to listen on a Unix
domain socket instead.`,
		// "dmr serve" runs the daemon in-process; it must not go through the
		// client-side Docker CLI plugin initialization used by every other
		// dmr subcommand.
		PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServe(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.port, "port", "", "TCP port to listen on (overrides MODEL_RUNNER_PORT; default 12434)")
	cmd.Flags().StringVar(&opts.socket, "socket", "", "Unix domain socket to listen on instead of TCP (overrides MODEL_RUNNER_SOCK)")
	cmd.Flags().StringVar(&opts.modelsPath, "models-path", "", "directory used to store pulled models (overrides MODELS_PATH)")
	cmd.MarkFlagsMutuallyExclusive("port", "socket")

	return cmd
}

func runServe(ctx context.Context, opts serveOptions) error {
	if err := applyServeEnv(opts); err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	return server.Run(ctx, server.Config{Version: Version})
}

// applyServeEnv translates serve flags into the environment variables read
// by pkg/envconfig, applying the standalone default TCP port when neither a
// flag nor an environment variable already specifies a listener.
//
// pkg/server.Run prefers MODEL_RUNNER_PORT over MODEL_RUNNER_SOCK whenever
// both are set, so an explicit --socket/--port flag must clear the other
// variable rather than merely setting its own — otherwise a
// MODEL_RUNNER_PORT inherited from the environment would silently override
// an explicit --socket flag (--port and --socket are themselves mutually
// exclusive as CLI flags; see MarkFlagsMutuallyExclusive in newServeCmd).
func applyServeEnv(opts serveOptions) error {
	switch {
	case opts.socket != "":
		if err := os.Unsetenv("MODEL_RUNNER_PORT"); err != nil {
			return fmt.Errorf("unable to unset MODEL_RUNNER_PORT: %w", err)
		}
		if err := os.Setenv("MODEL_RUNNER_SOCK", opts.socket); err != nil {
			return fmt.Errorf("unable to set MODEL_RUNNER_SOCK: %w", err)
		}
	case opts.port != "":
		if err := os.Unsetenv("MODEL_RUNNER_SOCK"); err != nil {
			return fmt.Errorf("unable to unset MODEL_RUNNER_SOCK: %w", err)
		}
		if err := os.Setenv("MODEL_RUNNER_PORT", opts.port); err != nil {
			return fmt.Errorf("unable to set MODEL_RUNNER_PORT: %w", err)
		}
	case os.Getenv("MODEL_RUNNER_PORT") == "" && os.Getenv("MODEL_RUNNER_SOCK") == "":
		if err := os.Setenv("MODEL_RUNNER_PORT", defaultPort); err != nil {
			return fmt.Errorf("unable to set MODEL_RUNNER_PORT: %w", err)
		}
	}

	if opts.modelsPath != "" {
		if err := os.Setenv("MODELS_PATH", opts.modelsPath); err != nil {
			return fmt.Errorf("unable to set MODELS_PATH: %w", err)
		}
	}

	return nil
}
