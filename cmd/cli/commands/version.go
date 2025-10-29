package commands

import (
	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "version",
		Short: "Show the Docker Model Runner version",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Printf("Docker Model Runner version %s\n", desktop.Version)
			cmd.Printf("Docker Engine Kind: %s\n", modelRunner.EngineKind())
		},
		ValidArgsFunction: completion.NoComplete,
	}
	return c
}
