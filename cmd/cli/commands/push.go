package commands

import (
	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/spf13/cobra"
)

func newPushCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "push MODEL",
		Short: "Push a model to Docker Hub",
		Args:  requireExactArgs(1, "push", "MODEL"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return pushModel(cmd, desktopClient, args[0])
		},
		ValidArgsFunction: completion.NoComplete,
	}
	return c
}

func pushModel(cmd *cobra.Command, desktopClient *desktop.Client, model string) error {
	printer := asPrinter(cmd)
	response, _, err := desktopClient.Push(model, printer)

	if err != nil {
		return handleClientError(err, "Failed to push model")
	}

	cmd.Println(response)
	return nil
}
