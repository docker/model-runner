package commands

import (
	"fmt"

	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/spf13/cobra"
)

func newStopCmd() *cobra.Command {
	var backend string

	const cmdArgs = "MODEL"
	c := &cobra.Command{
		Use:   "stop " + cmdArgs,
		Short: "Stop a running model",
		RunE: func(cmd *cobra.Command, args []string) error {
			model := models.NormalizeModelName(args[0])
			unloadResp, err := desktopClient.Unload(desktop.UnloadRequest{Backend: backend, Models: []string{model}})
			if err != nil {
				err = handleClientError(err, "Failed to stop model")
				return handleNotRunningError(err)
			}
			unloaded := unloadResp.UnloadedRunners
			if unloaded == 0 {
				cmd.Println("No such model running.")
			} else {
				cmd.Printf("Stopped %d model(s).\n", unloaded)
			}
			return nil
		},
		ValidArgsFunction: completion.NoComplete,
	}
	c.Args = func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf(
				"'docker model stop' requires MODEL.\\n\\n" +
					"Usage:  docker model stop " + cmdArgs + "\\n\\n" +
					"See 'docker model stop --help' for more information.",
			)
		}
		return nil
	}
	c.Flags().StringVar(&backend, "backend", "", "Optional backend to target")
	return c
}