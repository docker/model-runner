package commands

import (
	"fmt"

	"github.com/docker/model-runner/cmd/cli/commands/completion"

	"github.com/spf13/cobra"
)

func newRemoveCmd() *cobra.Command {
	var force bool

	c := &cobra.Command{
		Use:   "rm [MODEL...]",
		Short: "Remove local models downloaded from Docker Hub",
		Args: requireMinArgs(1, "rm", "[MODEL...]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := ensureStandaloneRunnerAvailable(cmd.Context(), asPrinter(cmd), false); err != nil {
				return fmt.Errorf("unable to initialize standalone model runner: %w", err)
			}
			response, err := desktopClient.Remove(args, force)
			if response != "" {
				cmd.Print(response)
			}
			if err != nil {
				return handleClientError(err, "Failed to remove model")
			}
			return nil
		},
		ValidArgsFunction: completion.ModelNames(getDesktopClient, -1),
	}

	c.Flags().BoolVarP(&force, "force", "f", false, "Forcefully remove the model")
	return c
}
