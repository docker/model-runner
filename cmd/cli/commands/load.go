package commands

import (
	"fmt"

	"github.com/docker/model-runner/cmd/cli/commands/completion"

	"github.com/spf13/cobra"
)

func newLoadCmd() *cobra.Command {
	const cmdArgs = "MODEL [MODEL ...]"
	c := &cobra.Command{
		Use:   "load " + cmdArgs,
		Short: "Load models into memory",
		RunE: func(cmd *cobra.Command, modelArgs []string) error {
			if len(modelArgs) == 0 {
				return fmt.Errorf(
					"'docker model load' requires at least one MODEL.\n\n" +
						"Usage:  docker model load " + cmdArgs + "\n\n" +
						"See 'docker model load --help' for more information.",
				)
			}

			// Load each model
			for _, model := range modelArgs {
				if err := desktopClient.LoadIntoMemory(model); err != nil {
					return handleClientError(err, fmt.Sprintf("Failed to load model %s", model))
				}
				cmd.Printf("Loaded model: %s\n", model)
			}

			return nil
		},
		ValidArgsFunction: completion.NoComplete,
	}
	return c
}

func newStartCmd() *cobra.Command {
	const cmdArgs = "MODEL [MODEL ...]"
	c := &cobra.Command{
		Use:   "start " + cmdArgs,
		Short: "Start models (alias for load)",
		RunE: func(cmd *cobra.Command, modelArgs []string) error {
			if len(modelArgs) == 0 {
				return fmt.Errorf(
					"'docker model start' requires at least one MODEL.\n\n" +
						"Usage:  docker model start " + cmdArgs + "\n\n" +
						"See 'docker model start --help' for more information.",
				)
			}

			// Load each model (start is an alias for load)
			for _, model := range modelArgs {
				if err := desktopClient.LoadIntoMemory(model); err != nil {
					return handleClientError(err, fmt.Sprintf("Failed to start model %s", model))
				}
				cmd.Printf("Started model: %s\n", model)
			}

			return nil
		},
		ValidArgsFunction: completion.NoComplete,
	}
	return c
}
