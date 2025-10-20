package commands

import (
	"fmt"

	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/cmd/cli/commands/formatter"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/spf13/cobra"
)

func newInspectCmd() *cobra.Command {
	var openai bool
	var remote bool
	c := &cobra.Command{
		Use:   "inspect MODEL",
		Short: "Display detailed information on one model",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf(
					"'docker model inspect' requires 1 argument.\n\n" +
						"Usage:  docker model inspect MODEL\n\n" +
						"See 'docker model inspect --help' for more information",
				)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := ensureStandaloneRunnerAvailable(cmd.Context(), cmd); err != nil {
				return fmt.Errorf("unable to initialize standalone model runner: %w", err)
			}
			if openai && remote {
				return fmt.Errorf("--remote flag cannot be used with --openai flag")
			}
			inspectedModel, err := inspectModel(args, openai, remote, desktopClient)
			if err != nil {
				return err
			}
			cmd.Print(inspectedModel)
			return nil
		},
		ValidArgsFunction: completion.ModelNames(getDesktopClient, 1),
	}
	c.Flags().BoolVar(&openai, "openai", false, "List model in an OpenAI format")
	c.Flags().BoolVarP(&remote, "remote", "r", false, "Show info for remote models")
	return c
}

func inspectModel(args []string, openai bool, remote bool, desktopClient *desktop.Client) (string, error) {
	// Normalize model name to add default org and tag if missing
	modelName := models.NormalizeModelName(args[0])
	if openai {
		model, err := desktopClient.InspectOpenAI(modelName)
		if err != nil {
			err = handleClientError(err, "Failed to get model "+modelName)
			return "", handleNotRunningError(err)
		}
		return formatter.ToStandardJSON(model)
	}
	model, err := desktopClient.Inspect(modelName, remote)
	if err != nil {
		err = handleClientError(err, "Failed to get model "+modelName)
		return "", handleNotRunningError(err)
	}
	return formatter.ToStandardJSON(model)
}
