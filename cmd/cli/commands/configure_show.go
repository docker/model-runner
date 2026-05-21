package commands

import (
	"encoding/json"
	"fmt"

	"github.com/docker/model-runner/cmd/cli/commands/completion"

	"github.com/spf13/cobra"
)

func newConfigureShowCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "show [MODEL]",
		Short: "Show effective runtime configuration for models",
		Long:  "Show the effective runtime configuration that Docker Model Runner has stored for one model or all models.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var modelFilter string
			if len(args) > 0 {
				modelFilter = args[0]
			}

			configs, err := desktopClient.ShowConfigs(modelFilter)
			if err != nil {
				return err
			}

			jsonResult, err := json.MarshalIndent(configs, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal configs to JSON: %w", err)
			}

			cmd.Println(string(jsonResult))
			return nil
		},
		ValidArgsFunction: completion.ModelNames(getDesktopClient, 1),
	}

	return c
}
