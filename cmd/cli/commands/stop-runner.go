package commands

import (
	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/spf13/cobra"
)

func newStopRunner() *cobra.Command {
	var models bool
	c := &cobra.Command{
		Use:   "stop-runner",
		Short: "Stop Docker Model Runner (Docker Engine only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUninstallOrStop(cmd, cleanupOptions{
				models:       models,
				removeImages: false,
			})
		},
		ValidArgsFunction: completion.NoComplete,
	}
	c.Flags().BoolVar(&models, "models", false, "Remove model storage volume")
	return c
}
