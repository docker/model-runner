// Command dmr is the unified Docker Model Runner daemon and client.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/docker/cli/cli/command"
	"github.com/docker/model-runner/cmd/cli/commands"
	"github.com/docker/model-runner/pkg/server"
	"github.com/spf13/cobra"
)

var Version = "dev"

const defaultHost = "http://localhost:12434"
const defaultPort = "12434"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cli, err := command.NewDockerCli()
	if err != nil {
		return fmt.Errorf("unable to initialize CLI: %w", err)
	}

	root := commands.NewRootCmd(cli)
	root.Use = "dmr"
	root.Short = "Docker Model Runner"

	if os.Getenv("MODEL_RUNNER_HOST") == "" {
		if err := os.Setenv("MODEL_RUNNER_HOST", defaultHost); err != nil {
			return fmt.Errorf("unable to set MODEL_RUNNER_HOST: %w", err)
		}
	}

	root.AddCommand(newServeCmd())

	return root.Execute()
}

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the Docker Model Runner daemon",
		// skip Docker CLI init; serve runs the daemon directly
		PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
		RunE: func(cmd *cobra.Command, _ []string) error {
			if os.Getenv("MODEL_RUNNER_PORT") == "" {
				if err := os.Setenv("MODEL_RUNNER_PORT", defaultPort); err != nil {
					return fmt.Errorf("unable to set MODEL_RUNNER_PORT: %w", err)
				}
			}

			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			return server.Run(ctx, server.Config{Version: Version})
		},
	}
}
