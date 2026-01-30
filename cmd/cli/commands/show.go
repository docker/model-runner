package commands

import (
	"fmt"
	"strings"
	"time"

	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/pkg/distribution/types"
	dmrm "github.com/docker/model-runner/pkg/inference/models"
	"github.com/spf13/cobra"
)

func newShowCmd() *cobra.Command {
	var remote bool
	c := &cobra.Command{
		Use:   "show MODEL",
		Short: "Show information for a model",
		Long:  "Display detailed information about a model in a human-readable format.",
		Args:  requireExactArgs(1, "show", "MODEL"),
		RunE: func(cmd *cobra.Command, args []string) error {
			output, err := showModel(args[0], remote, desktopClient)
			if err != nil {
				return err
			}
			cmd.Print(output)
			return nil
		},
		ValidArgsFunction: completion.ModelNames(getDesktopClient, 1),
	}
	c.Flags().BoolVarP(&remote, "remote", "r", false, "Show info for remote models")
	return c
}

func showModel(modelName string, remote bool, desktopClient *desktop.Client) (string, error) {
	model, err := desktopClient.Inspect(modelName, remote)
	if err != nil {
		return "", handleClientError(err, "Failed to get model "+modelName)
	}
	return formatModelInfo(model), nil
}

func formatModelInfo(model dmrm.Model) string {
	var sb strings.Builder

	// Model ID
	sb.WriteString(fmt.Sprintf("Model:       %s\n", model.ID))

	// Tags
	if len(model.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("Tags:        %s\n", strings.Join(model.Tags, ", ")))
	}

	// Created date
	if model.Created > 0 {
		created := time.Unix(model.Created, 0)
		sb.WriteString(fmt.Sprintf("Created:     %s\n", created.Format(time.RFC3339)))
	}

	// Config details
	if model.Config != nil {
		sb.WriteString("\n")

		if cfg, ok := model.Config.(*types.Config); ok {
			// Config fields using data-driven approach
			fields := []struct {
				label string
				value string
			}{
				{"Format:", string(cfg.Format)},
				{"Architecture:", cfg.Architecture},
				{"Parameters:", cfg.Parameters},
				{"Size:", cfg.Size},
				{"Quantization:", cfg.Quantization},
			}

			for _, field := range fields {
				if field.value != "" {
					sb.WriteString(fmt.Sprintf("%-14s%s\n", field.label, field.value))
				}
			}

			if cfg.ContextSize != nil {
				sb.WriteString(fmt.Sprintf("%-14s%d\n", "Context Size:", *cfg.ContextSize))
			}

			// Helper function to print metadata sections
			printMetadata := func(title string, data map[string]string) {
				if len(data) > 0 {
					sb.WriteString(fmt.Sprintf("\n%s:\n", title))
					for k, v := range data {
						sb.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
					}
				}
			}

			printMetadata("GGUF Metadata", cfg.GGUF)
			printMetadata("Safetensors Metadata", cfg.Safetensors)
			printMetadata("Diffusers Metadata", cfg.Diffusers)
		}
	}

	return sb.String()
}
