package commands

import (
	"fmt"

	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/spf13/cobra"
)

func newRemoveCmd() *cobra.Command {
	var force bool

	c := &cobra.Command{
		Use:   "rm [MODEL...]",
		Short: "Remove local models downloaded from Docker Hub",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf(
					"'docker model rm' requires at least 1 argument.\n\n" +
						"Usage:  docker model rm [MODEL...]\n\n" +
						"See 'docker model rm --help' for more information",
				)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := ensureStandaloneRunnerAvailable(cmd.Context(), cmd); err != nil {
				return fmt.Errorf("unable to initialize standalone model runner: %w", err)
			}
			// Normalize model names to add default org and tag if missing
			normalizedArgs := make([]string, len(args))
			for i, arg := range args {
				normalizedArgs[i] = models.NormalizeModelName(arg)
			}
			response, err := desktopClient.Remove(normalizedArgs, force)
			if response != "" {
				cmd.Print(response)
			}
			if err != nil {
				err = handleClientError(err, "Failed to remove model")
				return handleNotRunningError(err)
			}
			return nil
		},
		ValidArgsFunction: completion.ModelNames(getDesktopClient, -1),
	}

	c.Flags().BoolVarP(&force, "force", "f", false, "Forcefully remove the model")
	return c
}
