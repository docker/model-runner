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

// BoolPtrValue implements pflag.Value interface for *bool pointers
// This allows flags to have a nil default value to detect if explicitly set
type BoolPtrValue struct {
	ptr **bool
}

// NewBoolPtrValue creates a new BoolPtrValue for the given pointer
func NewBoolPtrValue(p **bool) *BoolPtrValue {
	return &BoolPtrValue{ptr: p}
}

func (v *BoolPtrValue) String() string {
	if v.ptr == nil || *v.ptr == nil {
		return ""
	}
	return strconv.FormatBool(**v.ptr)
}

func (v *BoolPtrValue) Set(s string) error {
	val, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	*v.ptr = &val
	return nil
}

func (v *BoolPtrValue) Type() string {
	return "bool"
}

func (v *BoolPtrValue) IsBoolFlag() bool {
	return true
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
	// Think parameter for reasoning models
	Think *bool
}

// RegisterFlags registers all configuration flags on the given cobra command.
func (f *ConfigureFlags) RegisterFlags(cmd *cobra.Command) {
	cmd.Flags().Var(NewInt32PtrValue(&f.ContextSize), "context-size", "context size (in tokens)")
	cmd.Flags().StringVar(&f.DraftModel, "speculative-draft-model", "", "draft model for speculative decoding")
	cmd.Flags().IntVar(&f.NumTokens, "speculative-num-tokens", 0, "number of tokens to predict speculatively")
	cmd.Flags().Float64Var(&f.MinAcceptanceRate, "speculative-min-acceptance-rate", 0, "minimum acceptance rate for speculative decoding")
	cmd.Flags().StringVar(&f.HFOverrides, "hf_overrides", "", "HuggingFace model config overrides (JSON) - vLLM only")
	cmd.Flags().Var(NewBoolPtrValue(&f.Think), "think", "enable reasoning mode for thinking models")
	cmd.Flags().StringVar(&f.Mode, "mode", "", "backend operation mode (completion, embedding, reranking)")
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

	// Set reasoning budget from --think flag
	reasoningBudget := f.getReasoningBudget()
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

// getReasoningBudget determines the reasoning budget from the --think flag.
// Returns nil if flag not set
// Returns -1 (unlimited) when --think or --think=true.
// Returns 0 (disabled) when --think=false.
func (f *ConfigureFlags) getReasoningBudget() *int32 {
	// If Think is nil, flag was not set - don't configure
	if f.Think == nil {
		return nil
	}
	// If explicitly set to true, enable reasoning (unlimited)
	if *f.Think {
		return ptr(reasoningBudgetUnlimited) // -1: reasoning enabled (unlimited)
	}
	// If explicitly set to false, disable reasoning
	return ptr(reasoningBudgetDisabled) // 0: reasoning disabled
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
