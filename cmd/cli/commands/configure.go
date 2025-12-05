package commands

import (
	"encoding/json"
	"fmt"

	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/pkg/inference"

	"github.com/docker/model-runner/pkg/inference/scheduling"
	"github.com/spf13/cobra"
)

func newConfigureCmd() *cobra.Command {
	var opts scheduling.ConfigureRequest
	var draftModel string
	var numTokens int
	var minAcceptanceRate float64
	var hfOverrides string
	var reasoningBudget int64

	c := &cobra.Command{
		Use:    "configure [--context-size=<n>] [--speculative-draft-model=<model>] [--hf_overrides=<json>] [--reasoning-budget=<n>] MODEL",
		Short:  "Configure runtime options for a model",
		Hidden: true,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf(
					"Exactly one model must be specified, got %d: %v\n\n"+
						"See 'docker model configure --help' for more information",
					len(args), args)
			}
			opts.Model = args[0]
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Build the speculative config if any speculative flags are set
			if draftModel != "" || numTokens > 0 || minAcceptanceRate > 0 {
				opts.Speculative = &inference.SpeculativeDecodingConfig{
					DraftModel:        draftModel,
					NumTokens:         numTokens,
					MinAcceptanceRate: minAcceptanceRate,
				}
			}
			// Parse and validate HuggingFace overrides if provided (vLLM-specific)
			if hfOverrides != "" {
				var hfo inference.HFOverrides
				if err := json.Unmarshal([]byte(hfOverrides), &hfo); err != nil {
					return fmt.Errorf("invalid --hf_overrides JSON: %w", err)
				}
				// Validate the overrides to prevent command injection
				if err := hfo.Validate(); err != nil {
					return err
				}
				if opts.VLLM == nil {
					opts.VLLM = &inference.VLLMConfig{}
				}
				opts.VLLM.HFOverrides = hfo
			}
			// Set llama.cpp-specific reasoning budget if explicitly provided
			// Note: We check if flag was changed rather than checking value > 0
			// because 0 is a valid value (disables reasoning) and -1 means unlimited
			if cmd.Flags().Changed("reasoning-budget") {
				if opts.LlamaCpp == nil {
					opts.LlamaCpp = &inference.LlamaCppConfig{}
				}
				opts.LlamaCpp.ReasoningBudget = &reasoningBudget
			}
			return desktopClient.ConfigureBackend(opts)
		},
		ValidArgsFunction: completion.ModelNames(getDesktopClient, -1),
	}

	c.Flags().Int64Var(&opts.ContextSize, "context-size", -1, "context size (in tokens)")
	c.Flags().StringVar(&draftModel, "speculative-draft-model", "", "draft model for speculative decoding")
	c.Flags().IntVar(&numTokens, "speculative-num-tokens", 0, "number of tokens to predict speculatively")
	c.Flags().Float64Var(&minAcceptanceRate, "speculative-min-acceptance-rate", 0, "minimum acceptance rate for speculative decoding")
	c.Flags().StringVar(&hfOverrides, "hf_overrides", "", "HuggingFace model config overrides (JSON) - vLLM only")
	c.Flags().Int64Var(&reasoningBudget, "reasoning-budget", 0, "reasoning budget for reasoning models - llama.cpp only")
	return c
}
