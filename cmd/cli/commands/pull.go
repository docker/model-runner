package commands

import (
	"fmt"
	"os"

	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

func newPullCmd() *cobra.Command {
	var ignoreRuntimeMemoryCheck bool
	var host string
	var port int

	c := &cobra.Command{
		Use:   "pull MODEL",
		Short: "Pull a model from Docker Hub or HuggingFace to your local environment",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf(
					"'docker model pull' requires 1 argument.\n\n" +
						"Usage:  docker model pull MODEL\n\n" +
						"See 'docker model pull --help' for more information",
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
			return pullModel(cmd, desktopClient, args[0], ignoreRuntimeMemoryCheck)
		},
		ValidArgsFunction: completion.NoComplete,
	}

	c.Flags().BoolVar(&ignoreRuntimeMemoryCheck, "ignore-runtime-memory-check", false, "Do not block pull if estimated runtime memory for model exceeds system resources.")
	c.Flags().StringVar(&host, "host", "", "Host address to bind Docker Model Runner (default \"127.0.0.1\")")
	c.Flags().IntVar(&port, "port", 0, "Docker container port for Docker Model Runner (default: 12434)")

	return c
}

func pullModel(cmd *cobra.Command, desktopClient *desktop.Client, model string, ignoreRuntimeMemoryCheck bool) error {
	// Normalize model name to add default org and tag if missing
	model = models.NormalizeModelName(model)
	var progress func(string)
	if isatty.IsTerminal(os.Stdout.Fd()) {
		progress = TUIProgress
	} else {
		progress = RawProgress
	}
	response, progressShown, err := desktopClient.Pull(model, ignoreRuntimeMemoryCheck, progress)

	// Add a newline before any output (success or error) if progress was shown.
	if progressShown {
		cmd.Println()
	}

	if err != nil {
		return handleNotRunningError(handleClientError(err, "Failed to pull model"))
	}

	cmd.Println(response)
	return nil
}

func TUIProgress(message string) {
	fmt.Print("\r\033[K", message)
}

func RawProgress(message string) {
	fmt.Println(message)
}
