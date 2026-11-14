package commands

import "github.com/spf13/cobra"

func newConfigCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "config",
		Short: "Manage persistent model runner configuration",
	}

	c.AddCommand(newSandboxConfigCmd())

	return c
}
