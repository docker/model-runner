package commands

import (
	"fmt"

	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/cmd/cli/desktop"

	"github.com/spf13/cobra"
)

func newPullCmd() *cobra.Command {
	var ignoreRuntimeMemoryCheck bool

	c := &cobra.Command{
		Use:   "pull MODEL",
		Short: "Pull a model from Docker Hub or HuggingFace to your local environment",
		Args: requireExactArgs(1, "pull", "MODEL"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := ensureStandaloneRunnerAvailable(cmd.Context(), asPrinter(cmd), false); err != nil {
				return fmt.Errorf("unable to initialize standalone model runner: %w", err)
			}
			return pullModel(cmd, desktopClient, args[0], ignoreRuntimeMemoryCheck)
		},
		ValidArgsFunction: completion.NoComplete,
	}

	c.Flags().BoolVar(&ignoreRuntimeMemoryCheck, "ignore-runtime-memory-check", false, "Do not block pull if estimated runtime memory for model exceeds system resources.")

	return c
}

func pullModel(cmd *cobra.Command, desktopClient *desktop.Client, model string, ignoreRuntimeMemoryCheck bool) error {
	printer := asPrinter(cmd)
	response, _, err := desktopClient.Pull(model, ignoreRuntimeMemoryCheck, printer)

	if err != nil {
		return handleClientError(err, "Failed to pull model")
	}

	cmd.Println(response)
	return nil
}
