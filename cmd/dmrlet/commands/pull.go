package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newPullCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pull MODEL",
		Short: "Pull a model without serving",
		Long: `Pull a model from Docker Hub or HuggingFace without starting an inference container.
This is useful for pre-downloading models.

Examples:
  dmrlet pull ai/smollm2
  dmrlet pull huggingface.co/microsoft/Phi-3-mini-4k-instruct-gguf`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPull(cmd, args[0])
		},
	}

	return cmd
}

func runPull(cmd *cobra.Command, modelRef string) error {
	ctx := cmd.Context()

	if err := initStore(); err != nil {
		return fmt.Errorf("initializing store: %w", err)
	}

	cmd.Printf("Pulling model: %s\n", modelRef)

	if err := store.EnsureModel(ctx, modelRef, os.Stdout); err != nil {
		return fmt.Errorf("pulling model: %w", err)
	}

	cmd.Printf("\nModel pulled successfully: %s\n", modelRef)
	return nil
}
