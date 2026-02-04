package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop MODEL",
		Short: "Stop a running model",
		Long: `Stop a running model and remove its container.

Examples:
  dmrlet stop ai/smollm2`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStop(cmd, args[0])
		},
	}

	return cmd
}

func runStop(cmd *cobra.Command, modelRef string) error {
	ctx := cmd.Context()

	if err := initManager(ctx); err != nil {
		return fmt.Errorf("initializing manager: %w", err)
	}

	if err := manager.Stop(ctx, modelRef); err != nil {
		return fmt.Errorf("stopping model: %w", err)
	}

	cmd.Printf("Stopped model: %s\n", modelRef)
	return nil
}
