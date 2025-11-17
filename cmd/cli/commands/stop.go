package commands

import (
	"fmt"

	"github.com/docker/model-runner/cmd/cli/commands/completion"

	"github.com/spf13/cobra"
)

func newStopCmd() *cobra.Command {
	const cmdArgs = "MODEL [MODEL ...]"
	c := &cobra.Command{
		Use:   "stop " + cmdArgs,
		Short: "Stop models (alias for unload)",
		RunE: func(cmd *cobra.Command, modelArgs []string) error {
			if len(modelArgs) == 0 {
				return fmt.Errorf(
					"'docker model stop' requires at least one MODEL.\n\n" +
						"Usage:  docker model stop " + cmdArgs + "\n\n" +
						"See 'docker model stop --help' for more information.",
				)
			}

			// Unload each model (stop is an alias for unload)
			for _, model := range modelArgs {
				if err := desktopClient.UnloadFromMemory(model); err != nil {
					return handleClientError(err, fmt.Sprintf("Failed to stop model %s", model))
				}
				cmd.Printf("Stopped model: %s\n", model)
			}

			return nil
		},
		ValidArgsFunction: completion.NoComplete,
	}
	return c
}
