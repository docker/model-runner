package commands

import (
	"fmt"

	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/spf13/cobra"
)

func newPushCmd() *cobra.Command {
	var host string
	var port int

	c := &cobra.Command{
		Use:   "push MODEL",
		Short: "Push a model to Docker Hub",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf(
					"'docker model push' requires 1 argument.\n\n" +
						"Usage:  docker model push MODEL\n\n" +
						"See 'docker model push --help' for more information",
				)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Override model runner context if host/port is specified
			if host != "" || port != 0 {
				if err := overrideModelRunnerContext(host, port); err != nil {
					return err
				}
			}

			if _, err := ensureStandaloneRunnerAvailable(cmd.Context(), cmd); err != nil {
				return fmt.Errorf("unable to initialize standalone model runner: %w", err)
			}
			return pushModel(cmd, desktopClient, args[0])
		},
		ValidArgsFunction: completion.NoComplete,
	}

	c.Flags().StringVar(&host, "host", "", "Host address to bind Docker Model Runner (default \"127.0.0.1\")")
	c.Flags().IntVar(&port, "port", 0, "Docker container port for Docker Model Runner (default: 12434)")

	return c
}

func pushModel(cmd *cobra.Command, desktopClient *desktop.Client, model string) error {
	// Normalize model name to add default org and tag if missing
	model = models.NormalizeModelName(model)
	response, progressShown, err := desktopClient.Push(model, TUIProgress)

	// Add a newline before any output (success or error) if progress was shown.
	if progressShown {
		cmd.Println()
	}

	if err != nil {
		return handleNotRunningError(handleClientError(err, "Failed to push model"))
	}

	cmd.Println(response)
	return nil
}
