package commands

import (
	"runtime"

	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "version",
		Short: "Show the Docker Model Runner version",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Println("Client:")
			cmd.Printf(" Version:    %s\n", desktop.Version)
			cmd.Printf(" OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)

			cmd.Println()
			cmd.Println("Server:")
			if desktopClient == nil {
				cmd.Println(" Version:    (not reachable)")
				cmd.Println(" Engine:     (not reachable)")
				return
			}
			sv, err := desktopClient.ServerVersion()
			if err != nil {
				cmd.Println(" Version:    (not reachable)")
			} else {
				cmd.Printf(" Version:    %s\n", sv.Version)
			}
			cmd.Printf(" Engine:     %s\n", modelRunner.EngineKind())
		},
		ValidArgsFunction: completion.NoComplete,
	}
	return c
}
