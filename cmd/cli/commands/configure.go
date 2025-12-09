package commands

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/pkg/inference"

	"github.com/docker/model-runner/pkg/inference/scheduling"
	"github.com/spf13/cobra"
)

// Int32PtrValue implements pflag.Value interface for *int32 pointers
// This allows flags to have a nil default value instead of 0
type Int32PtrValue struct {
	ptr **int32
}

func NewInt32PtrValue(p **int32) *Int32PtrValue {
	return &Int32PtrValue{ptr: p}
}

func (v *Int32PtrValue) String() string {
	if v.ptr == nil || *v.ptr == nil {
		return ""
	}
	return strconv.FormatInt(int64(**v.ptr), 10)
}

func (v *Int32PtrValue) Set(s string) error {
	val, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return err
	}
	i32 := int32(val)
	*v.ptr = &i32
	return nil
}

func (v *Int32PtrValue) Type() string {
	return "int32"
}

func newConfigureCmd() *cobra.Command {
	var opts scheduling.ConfigureRequest
	var draftModel string
	var numTokens int
	var minAcceptanceRate float64
	var hfOverrides string
	var contextSize *int32
	var reasoningBudget *int32

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
			// contextSize is nil by default, only set if user provided the flag
			opts.ContextSize = contextSize
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
			// Set llama.cpp-specific reasoning budget if provided
			// reasoningBudget is nil by default, only set if user provided the flag
			if reasoningBudget != nil {
				if opts.LlamaCpp == nil {
					opts.LlamaCpp = &inference.LlamaCppConfig{}
				}
				opts.LlamaCpp.ReasoningBudget = reasoningBudget
			}
			return desktopClient.ConfigureBackend(opts)
		},
		ValidArgsFunction: completion.ModelNames(getDesktopClient, -1),
	}

	c.Flags().Var(NewInt32PtrValue(&contextSize), "context-size", "context size (in tokens)")
	c.Flags().StringVar(&draftModel, "speculative-draft-model", "", "draft model for speculative decoding")
	c.Flags().IntVar(&numTokens, "speculative-num-tokens", 0, "number of tokens to predict speculatively")
	c.Flags().Float64Var(&minAcceptanceRate, "speculative-min-acceptance-rate", 0, "minimum acceptance rate for speculative decoding")
	c.Flags().StringVar(&hfOverrides, "hf_overrides", "", "HuggingFace model config overrides (JSON) - vLLM only")
	c.Flags().Var(NewInt32PtrValue(&reasoningBudget), "reasoning-budget", "reasoning budget for reasoning models - llama.cpp only")
	return c
}
