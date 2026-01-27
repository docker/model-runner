package inference

import "strings"

// LlamaCppAllowedFlags contains safe flags for llama.cpp server
var LlamaCppAllowedFlags = map[string]bool{
	// Threading and performance
	"-t": true, "--threads": true,
	"-tb": true, "--threads-batch": true,

	// Context and batching
	"-c": true, "--ctx-size": true,
	"-n": true, "--n-predict": true,
	"-b": true, "--batch-size": true,
	"-ub": true, "--ubatch-size": true,

	// Sampling parameters
	"--temp": true, "--temperature": true,
	"--top-k": true, "--top-p": true, "--min-p": true,
	"--repeat-last-n": true, "--repeat-penalty": true,
	"--presence-penalty": true, "--frequency-penalty": true,
	"--seed": true, "-s": true,

	// GPU and memory
	"-ngl": true, "--gpu-layers": true, "--n-gpu-layers": true,
	"-sm": true, "--split-mode": true,
	"-ts": true, "--tensor-split": true,
	"-mg": true, "--main-gpu": true,
	"--mlock": true, "--mmap": true, "--no-mmap": true,

	// Server settings
	"-np": true, "--parallel": true,
	"--timeout": true, "-to": true,
	"-cb": true, "--cont-batching": true,
	"-fa": true, "--flash-attn": true,
	"--cache-prompt": true,

	// KV cache quantization
	"--cache-type-k": true, "--cache-type-v": true,

	// Mode flags (already handled but safe to allow)
	"--embeddings": true, "--embedding": true,
	"--reranking": true,
	"--metrics":   true, "--no-metrics": true,
	"--jinja": true,
	"-v":      true, "--verbose": true,
	"--reasoning-budget": true,

	// RoPE scaling
	"--rope-scaling": true, "--rope-scale": true,
	"--rope-freq-base": true, "--rope-freq-scale": true,
}

// VLLMAllowedFlags contains safe flags for vLLM engine
var VLLMAllowedFlags = map[string]bool{
	// Parallelism
	"--tensor-parallel-size": true, "-tp": true,
	"--pipeline-parallel-size": true, "-pp": true,

	// Model configuration
	"--max-model-len":          true,
	"--max-num-batched-tokens": true,
	"--max-num-seqs":           true,
	"--block-size":             true,
	"--swap-space":             true,
	"--seed":                   true,

	// Data types and quantization
	"--dtype":          true,
	"--quantization":   true,
	"-q":               true,
	"--kv-cache-dtype": true,

	// Performance flags
	"--enforce-eager":             true,
	"--enable-prefix-caching":     true,
	"--enable-chunked-prefill":    true,
	"--disable-custom-all-reduce": true,
	"--use-v2-block-manager":      true,

	// Tokenizer
	"--tokenizer-mode":    true,
	"--trust-remote-code": true,
	"--max-logprobs":      true,

	// Misc
	"--revision":          true,
	"--load-format":       true,
	"--disable-log-stats": true,
	"--served-model-name": true,
}

// AllowedFlags maps backend names to their allowed flag keys
var AllowedFlags = map[string]map[string]bool{
	"llama.cpp": LlamaCppAllowedFlags,
	"vllm":      VLLMAllowedFlags,
}

// ParseFlagKey extracts the flag key from a flag string.
// "--threads=4" -> "--threads", "-t" -> "-t", "4" -> ""
func ParseFlagKey(flag string) string {
	if !strings.HasPrefix(flag, "-") {
		return "" // Not a flag, it's a value
	}
	if idx := strings.Index(flag, "="); idx != -1 {
		return flag[:idx]
	}
	return flag
}

// GetAllowedFlags returns the allowlist for a backend, or nil if unknown
func GetAllowedFlags(backendName string) map[string]bool {
	return AllowedFlags[backendName]
}
