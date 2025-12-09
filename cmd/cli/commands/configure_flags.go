package commands

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/scheduling"
	"github.com/spf13/cobra"
)

// Reasoning budget constants for the think parameter conversion
const (
	reasoningBudgetUnlimited int32 = -1
	reasoningBudgetDisabled  int32 = 0
	reasoningBudgetMedium    int32 = 1024
	reasoningBudgetLow       int32 = 256
)

// Int32PtrValue implements pflag.Value interface for *int32 pointers
// This allows flags to have a nil default value instead of 0
type Int32PtrValue struct {
	ptr **int32
}

// NewInt32PtrValue creates a new Int32PtrValue for the given pointer
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

// ptr is a helper function to create a pointer to int32
func ptr(v int32) *int32 {
	return &v
}

// ConfigureFlags holds all the flags for configuring a model backend
type ConfigureFlags struct {
	// Backend mode (completion, embedding, reranking)
	Mode string
	// ContextSize is the context size in tokens
	ContextSize *int32
	// Speculative decoding flags
	DraftModel        string
	NumTokens         int
	MinAcceptanceRate float64
	// vLLM-specific flags
	HFOverrides string
	// llama.cpp-specific flags
	ReasoningBudget *int32
	// Think parameter for reasoning models (true/false/high/medium/low)
	Think string
}

// RegisterFlags registers all configuration flags on the given cobra command.
// This ensures both configure and compose commands have the same flags.
func (f *ConfigureFlags) RegisterFlags(cmd *cobra.Command) {
	cmd.Flags().Var(NewInt32PtrValue(&f.ContextSize), "context-size", "context size (in tokens)")
	cmd.Flags().StringVar(&f.DraftModel, "speculative-draft-model", "", "draft model for speculative decoding")
	cmd.Flags().IntVar(&f.NumTokens, "speculative-num-tokens", 0, "number of tokens to predict speculatively")
	cmd.Flags().Float64Var(&f.MinAcceptanceRate, "speculative-min-acceptance-rate", 0, "minimum acceptance rate for speculative decoding")
	cmd.Flags().StringVar(&f.HFOverrides, "hf_overrides", "", "HuggingFace model config overrides (JSON) - vLLM only")
	cmd.Flags().Var(NewInt32PtrValue(&f.ReasoningBudget), "reasoning-budget", "reasoning budget for reasoning models - llama.cpp only")
	cmd.Flags().StringVar(&f.Mode, "mode", "", "backend operation mode (completion, embedding, reranking)")
	cmd.Flags().StringVar(&f.Think, "think", "", "enable reasoning mode for thinking models (true/false/high/medium/low)")
}

// BuildConfigureRequest builds a scheduling.ConfigureRequest from the flags.
// The model parameter is the model name to configure.
func (f *ConfigureFlags) BuildConfigureRequest(model string) (scheduling.ConfigureRequest, error) {
	req := scheduling.ConfigureRequest{
		Model: model,
	}

	// Set context size
	req.ContextSize = f.ContextSize

	// Build speculative config if any speculative flags are set
	if f.DraftModel != "" || f.NumTokens > 0 || f.MinAcceptanceRate > 0 {
		req.Speculative = &inference.SpeculativeDecodingConfig{
			DraftModel:        f.DraftModel,
			NumTokens:         f.NumTokens,
			MinAcceptanceRate: f.MinAcceptanceRate,
		}
	}

	// Parse and validate HuggingFace overrides if provided (vLLM-specific)
	if f.HFOverrides != "" {
		var hfo inference.HFOverrides
		if err := json.Unmarshal([]byte(f.HFOverrides), &hfo); err != nil {
			return req, fmt.Errorf("invalid --hf_overrides JSON: %w", err)
		}
		// Validate the overrides to prevent command injection
		if err := hfo.Validate(); err != nil {
			return req, err
		}
		if req.VLLM == nil {
			req.VLLM = &inference.VLLMConfig{}
		}
		req.VLLM.HFOverrides = hfo
	}

	// Determine reasoning budget - either from --reasoning-budget or --think
	reasoningBudget, err := f.getReasoningBudget()
	if err != nil {
		return req, err
	}
	if reasoningBudget != nil {
		if req.LlamaCpp == nil {
			req.LlamaCpp = &inference.LlamaCppConfig{}
		}
		req.LlamaCpp.ReasoningBudget = reasoningBudget
	}

	// Parse mode if provided
	if f.Mode != "" {
		parsedMode, err := parseBackendMode(f.Mode)
		if err != nil {
			return req, err
		}
		req.Mode = &parsedMode
	}

	return req, nil
}

// getReasoningBudget determines the reasoning budget from either --reasoning-budget or --think flags.
// Returns an error if both flags are provided, as they are mutually exclusive.
func (f *ConfigureFlags) getReasoningBudget() (*int32, error) {
	// Check for mutual exclusivity - both flags cannot be set at the same time
	if f.ReasoningBudget != nil && f.Think != "" {
		return nil, fmt.Errorf("--think and --reasoning-budget are mutually exclusive; please use only one")
	}

	// If reasoning-budget is explicitly set, use it
	if f.ReasoningBudget != nil {
		return f.ReasoningBudget, nil
	}

	// Otherwise, parse think parameter
	if f.Think != "" {
		return parseThinkToReasoningBudget(f.Think)
	}

	return nil, nil
}

// parseBackendMode parses a string mode value into an inference.BackendMode.
func parseBackendMode(mode string) (inference.BackendMode, error) {
	switch strings.ToLower(mode) {
	case "completion":
		return inference.BackendModeCompletion, nil
	case "embedding":
		return inference.BackendModeEmbedding, nil
	case "reranking":
		return inference.BackendModeReranking, nil
	default:
		return inference.BackendModeCompletion, fmt.Errorf("invalid mode %q: must be one of completion, embedding, reranking", mode)
	}
}

// parseThinkToReasoningBudget converts the think parameter string to a reasoning budget value.
// Accepts: "true", "false", "high", "medium", "low"
// Returns:
//   - nil for empty string or "true" (use server default, which is unlimited)
//   - -1 for "high" (explicitly set unlimited)
//   - 0 for "false" (disable thinking)
//   - 1024 for "medium"
//   - 256 for "low"
func parseThinkToReasoningBudget(think string) (*int32, error) {
	if think == "" {
		return nil, nil
	}

	switch strings.ToLower(think) {
	case "true":
		// Use nil to let the server use its default (currently unlimited)
		return nil, nil
	case "high":
		// Explicitly set unlimited reasoning budget
		return ptr(reasoningBudgetUnlimited), nil
	case "false":
		return ptr(reasoningBudgetDisabled), nil
	case "medium":
		return ptr(reasoningBudgetMedium), nil
	case "low":
		return ptr(reasoningBudgetLow), nil
	default:
		return nil, fmt.Errorf("invalid think value %q: must be one of true, false, high, medium, low", think)
	}
}
