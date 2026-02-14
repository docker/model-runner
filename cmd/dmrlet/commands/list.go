package commands

import (
	"fmt"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
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

	table := tablewriter.NewTable(os.Stdout,
		tablewriter.WithRenderer(renderer.NewBlueprint(tw.Rendition{
			Borders: tw.BorderNone,
			Settings: tw.Settings{
				Separators: tw.Separators{
					BetweenColumns: tw.Off,
				},
				Lines: tw.Lines{
					ShowHeaderLine: tw.Off,
				},
			},
		})),
		tablewriter.WithConfig(tablewriter.Config{
			Header: tw.CellConfig{
				Formatting: tw.CellFormatting{
					AutoFormat: tw.Off,
				},
				Alignment: tw.CellAlignment{Global: tw.AlignLeft},
				Padding:   tw.CellPadding{Global: tw.Padding{Left: "", Right: "  "}},
			},
			Row: tw.CellConfig{
				Alignment: tw.CellAlignment{Global: tw.AlignLeft},
				Padding:   tw.CellPadding{Global: tw.Padding{Left: "", Right: "  "}},
			},
		}),
	)
	table.Header([]string{"MODEL", "BACKEND", "PORT", "ENDPOINT"})

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
