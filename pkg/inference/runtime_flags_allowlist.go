package inference

import "strings"

// LlamaCppAllowedFlags contains safe flags for llama.cpp server.
// This list is based on llama.cpp server documentation.
// Flags involving file paths are intentionally excluded for security.
var LlamaCppAllowedFlags = map[string]bool{
	// Threading and CPU control
	"-t": true, "--threads": true,
	"-tb": true, "--threads-batch": true,
	"-C": true, "--cpu-mask": true,
	"-Cr": true, "--cpu-range": true,
	"--cpu-strict": true,
	"--prio":       true,
	"--poll":       true,
	"-Cb":          true, "--cpu-mask-batch": true,
	"-Crb": true, "--cpu-range-batch": true,
	"--cpu-strict-batch": true,
	"--prio-batch":       true,
	"--poll-batch":       true,

	// Context and prediction
	"-c": true, "--ctx-size": true,
	"-n": true, "--predict": true, "--n-predict": true,
	"--keep": true,

	// Batching and performance
	"-b": true, "--batch-size": true,
	"-ub": true, "--ubatch-size": true,
	"--swa-full": true,
	"-fa":        true, "--flash-attn": true,
	"--perf": true, "--no-perf": true,

	// Sampling parameters
	"--samplers": true,
	"-s":         true, "--seed": true,
	"--temp": true, "--temperature": true,
	"--top-k":              true,
	"--top-p":              true,
	"--min-p":              true,
	"--top-nsigma":         true,
	"--xtc-probability":    true,
	"--xtc-threshold":      true,
	"--typical":            true,
	"--repeat-last-n":      true,
	"--repeat-penalty":     true,
	"--presence-penalty":   true,
	"--frequency-penalty":  true,
	"--dry-multiplier":     true,
	"--dry-base":           true,
	"--dry-allowed-length": true,
	"--dry-penalty-last-n": true,
	"--mirostat":           true,
	"--mirostat-lr":        true,
	"--mirostat-ent":       true,
	"--ignore-eos":         true,
	"--dynatemp-range":     true,
	"--dynatemp-exp":       true,

	// GPU and device management
	"-dev": true, "--device": true,
	"-ngl": true, "--gpu-layers": true, "--n-gpu-layers": true,
	"-sm": true, "--split-mode": true,
	"-ts": true, "--tensor-split": true,
	"-mg": true, "--main-gpu": true,
	"-fit": true, "--fit": true,
	"-fitt": true, "--fit-target": true,
	"-fitc": true, "--fit-ctx": true,

	// Memory and caching
	"-kvo": true, "--kv-offload": true,
	"-nkvo": true, "--no-kv-offload": true,
	"--repack": true, "-nr": true, "--no-repack": true,
	"--no-host": true,
	"-ctk":      true, "--cache-type-k": true,
	"-ctv": true, "--cache-type-v": true,
	"--mlock": true,
	"--mmap":  true, "--no-mmap": true,
	"-dio": true, "--direct-io": true,
	"-ndio": true, "--no-direct-io": true,
	"-cram": true, "--cache-ram": true,
	"-kvu": true, "--kv-unified": true,
	"--context-shift": true, "--no-context-shift": true,

	// RoPE scaling
	"--rope-scaling":     true,
	"--rope-scale":       true,
	"--rope-freq-base":   true,
	"--rope-freq-scale":  true,
	"--yarn-orig-ctx":    true,
	"--yarn-ext-factor":  true,
	"--yarn-attn-factor": true,
	"--yarn-beta-slow":   true,
	"--yarn-beta-fast":   true,

	// Server configuration
	"-np": true, "--parallel": true,
	"-cb": true, "--cont-batching": true,
	"-nocb": true, "--no-cont-batching": true,
	"--warmup": true, "--no-warmup": true,
	"-to": true, "--timeout": true,
	"--threads-http":       true,
	"--cache-prompt":       true,
	"--no-cache-prompt":    true,
	"--cache-reuse":        true,
	"--sleep-idle-seconds": true,

	// Multimodal (safe flags only - no file paths)
	"--mmproj-auto": true, "--no-mmproj": true, "--no-mmproj-auto": true,
	"--mmproj-offload": true, "--no-mmproj-offload": true,
	"--image-min-tokens": true,
	"--image-max-tokens": true,
	"--spm-infill":       true,

	// Speculative decoding (safe flags only - no file paths)
	"--draft": true, "--draft-n": true, "--draft-max": true,
	"--draft-min": true, "--draft-n-min": true,
	"--draft-p-min": true,
	"-cd":           true, "--ctx-size-draft": true,
	"-devd": true, "--device-draft": true,
	"-ngld": true, "--gpu-layers-draft": true, "--n-gpu-layers-draft": true,
	"-td": true, "--threads-draft": true,
	"-tbd": true, "--threads-batch-draft": true,

	// LoRA (safe flags only - no file paths)
	"--lora-init-without-apply": true,

	// Control vectors (safe flags only - no file paths)
	"--control-vector-layer-range": true,

	// Grammar and constraints (safe flags only - no file paths)
	"--grammar": true,
	"-j":        true, "--json-schema": true,
	"-bs": true, "--backend-sampling": true,

	// Template and format control (safe flags only - no file paths)
	"--chat-template":        true,
	"--chat-template-kwargs": true,
	"--jinja":                true, "--no-jinja": true,
	"--pooling":              true,
	"--reasoning-format":     true,
	"--reasoning-budget":     true,
	"--prefill-assistant":    true,
	"--no-prefill-assistant": true,

	// Web interface and API (safe flags only - no file paths)
	"--api-prefix": true,
	"--webui":      true, "--no-webui": true,
	"--webui-config": true,
	"--api-key":      true,
	"--metrics":      true,
	"--no-metrics":   true,
	"--props":        true,
	"--slots":        true, "--no-slots": true,

	// Embedding and specialized
	"--embedding": true, "--embeddings": true,
	"--rerank": true, "--reranking": true,
	"-sps": true, "--slot-prompt-similarity": true,

	// Tensor and computation (safe flags only)
	"-cmoe": true, "--cpu-moe": true,
	"-ncmoe": true, "--n-cpu-moe": true,
	"--check-tensors": true,
	"--op-offload":    true, "--no-op-offload": true,

	// Verbose/debug
	"-v": true, "--verbose": true,
}

// VLLMAllowedFlags contains safe flags for vLLM engine.
// Flags involving file paths are intentionally excluded for security.
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

	// GPU memory
	"--gpu-memory-utilization": true,
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
