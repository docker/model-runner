package commands

import (
	"fmt"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List running models",
		Long: `List all running inference models managed by dmrlet.

Examples:
  dmrlet list
  dmrlet ls`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd)
		},
	}

	return cmd
}

func runList(cmd *cobra.Command) error {
	ctx := cmd.Context()

	if err := initManager(ctx); err != nil {
		return fmt.Errorf("initializing manager: %w", err)
	}

	running, err := manager.List(ctx)
	if err != nil {
		return fmt.Errorf("listing models: %w", err)
	}

	if len(running) == 0 {
		cmd.Println("No running models")
		return nil
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"MODEL", "BACKEND", "PORT", "ENDPOINT"})

	for _, m := range running {
		table.Append([]string{
			m.ModelRef,
			string(m.Backend),
			fmt.Sprintf("%d", m.Port),
			m.Endpoint,
		})
	}

	table.Render()
	return nil
}
