package commands

import (
	"bytes"
	"strings"
	"time"

	"github.com/docker/go-units"
	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/spf13/cobra"
)

func newPSCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "ps",
		Short: "List running models",
		RunE: func(cmd *cobra.Command, args []string) error {
			ps, err := desktopClient.PS()
			if err != nil {
				return handleClientError(err, "Failed to list running models")
			}
			cmd.Print(psTable(ps))
			return nil
		},
		ValidArgsFunction: completion.NoComplete,
	}
	return c
}

func psTable(ps []desktop.BackendStatus) string {
	var buf bytes.Buffer
	table := newTable(&buf)
	table.Header([]string{"MODEL NAME", "BACKEND", "MODE", "UNTIL"})

	for _, status := range ps {
		modelName := status.ModelName
		if strings.HasPrefix(modelName, "sha256:") {
			modelName = modelName[7:19]
		} else {
			modelName = stripDefaultsFromModelName(modelName)
		}

		table.Append([]string{
			modelName,
			status.BackendName,
			status.Mode,
			formatUntil(status),
		})
	}

	table.Render()
	return buf.String()
}

func formatUntil(status desktop.BackendStatus) string {
	keepAlive := inference.KeepAliveDefault
	if status.KeepAlive != nil {
		keepAlive = *status.KeepAlive
	}

	if keepAlive == inference.KeepAliveForever {
		return "Forever"
	}

	if status.InUse || status.LastUsed.IsZero() {
		return units.HumanDuration(keepAlive.Duration()) + " from now"
	}

	remaining := keepAlive.Duration() - time.Since(status.LastUsed)
	if remaining <= 0 {
		return "Expiring"
	}
	return units.HumanDuration(remaining) + " from now"
}
