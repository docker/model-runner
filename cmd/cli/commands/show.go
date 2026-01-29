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
			if cfg.Format != "" {
				sb.WriteString(fmt.Sprintf("Format:       %s\n", cfg.Format))
			}
			if cfg.Architecture != "" {
				sb.WriteString(fmt.Sprintf("Architecture: %s\n", cfg.Architecture))
			}
			if cfg.Parameters != "" {
				sb.WriteString(fmt.Sprintf("Parameters:   %s\n", cfg.Parameters))
			}
			if cfg.Size != "" {
				sb.WriteString(fmt.Sprintf("Size:         %s\n", cfg.Size))
			}
			if cfg.Quantization != "" {
				sb.WriteString(fmt.Sprintf("Quantization: %s\n", cfg.Quantization))
			}
			if cfg.ContextSize != nil {
				sb.WriteString(fmt.Sprintf("Context Size: %d\n", *cfg.ContextSize))
			}

			// GGUF metadata
			if len(cfg.GGUF) > 0 {
				sb.WriteString("\nGGUF Metadata:\n")
				for k, v := range cfg.GGUF {
					sb.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
				}
			}

			// Safetensors metadata
			if len(cfg.Safetensors) > 0 {
				sb.WriteString("\nSafetensors Metadata:\n")
				for k, v := range cfg.Safetensors {
					sb.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
				}
			}

			// Diffusers metadata
			if len(cfg.Diffusers) > 0 {
				sb.WriteString("\nDiffusers Metadata:\n")
				for k, v := range cfg.Diffusers {
					sb.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
				}
			}
		}
	}

	return sb.String()
}
