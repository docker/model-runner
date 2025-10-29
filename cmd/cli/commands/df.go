package commands

import (
	"bytes"

	"github.com/docker/go-units"
	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

func newDFCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "df",
		Short: "Show Docker Model Runner disk usage",
		RunE: func(cmd *cobra.Command, args []string) error {
			df, err := desktopClient.DF()
			if err != nil {
				return handleClientError(err, "Failed to list running models")
			}
			cmd.Print(diskUsageTable(df))
			return nil
		},
		ValidArgsFunction: completion.NoComplete,
	}
	return c
}

func diskUsageTable(df desktop.DiskUsage) string {
	var buf bytes.Buffer
	table := tablewriter.NewWriter(&buf)

	table.SetHeader([]string{"TYPE", "SIZE"})

	table.SetBorder(false)
	table.SetColumnSeparator("")
	table.SetHeaderLine(false)
	table.SetTablePadding("  ")
	table.SetNoWhiteSpace(true)

	table.SetColumnAlignment([]int{
		tablewriter.ALIGN_LEFT, // TYPE
		tablewriter.ALIGN_LEFT, // SIZE
	})
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)

	table.Append([]string{"Models", units.CustomSize("%.2f%s", float64(df.ModelsDiskUsage), 1000.0, []string{"B", "kB", "MB", "GB", "TB", "PB", "EB", "ZB", "YB"})})
	if df.DefaultBackendDiskUsage != 0 {
		table.Append([]string{"Inference engine", units.CustomSize("%.2f%s", float64(df.DefaultBackendDiskUsage), 1000.0, []string{"B", "kB", "MB", "GB", "TB", "PB", "EB", "ZB", "YB"})})
	}

	table.Render()
	return buf.String()
}
