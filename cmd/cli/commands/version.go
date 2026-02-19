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
			serverVersion := "(not reachable)"
			if desktopClient != nil {
				if sv, err := desktopClient.ServerVersion(); err == nil {
					serverVersion = sv.Version
				}
			}
			cmd.Printf(" Version:    %s\n", serverVersion)
			if modelRunner != nil {
				cmd.Printf(" Engine:     %s\n", modelRunner.EngineKind())
			} else {
				cmd.Println(" Engine:     (not reachable)")
			}
		},
		ValidArgsFunction: completion.NoComplete,
	}
	return c
}
