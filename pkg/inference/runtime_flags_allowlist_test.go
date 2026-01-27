package inference

import (
	"testing"
)

func TestParseFlagKey(t *testing.T) {
	tests := []struct {
		name     string
		flag     string
		expected string
	}{
		{
			name:     "long flag",
			flag:     "--threads",
			expected: "--threads",
		},
		{
			name:     "short flag",
			flag:     "-t",
			expected: "-t",
		},
		{
			name:     "long flag with equals",
			flag:     "--threads=4",
			expected: "--threads",
		},
		{
			name:     "short flag with equals",
			flag:     "-t=4",
			expected: "-t",
		},
		{
			name:     "value only (number)",
			flag:     "4",
			expected: "",
		},
		{
			name:     "value only (string)",
			flag:     "some-value",
			expected: "",
		},
		{
			name:     "empty string",
			flag:     "",
			expected: "",
		},
		{
			name:     "long flag with complex value",
			flag:     "--model-name=llama-3.2-1b",
			expected: "--model-name",
		},
		{
			name:     "flag with multiple equals",
			flag:     "--config=key=value",
			expected: "--config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseFlagKey(tt.flag)
			if result != tt.expected {
				t.Errorf("ParseFlagKey(%q) = %q, want %q", tt.flag, result, tt.expected)
			}
		})
	}
}

func TestGetAllowedFlags(t *testing.T) {
	tests := []struct {
		name       string
		backend    string
		expectNil  bool
		checkFlags []string // flags that should be in the allowlist
	}{
		{
			name:       "llama.cpp backend",
			backend:    "llama.cpp",
			expectNil:  false,
			checkFlags: []string{"--threads", "-t", "--ctx-size", "-ngl", "--verbose", "-v"},
		},
		{
			name:       "vllm backend",
			backend:    "vllm",
			expectNil:  false,
			checkFlags: []string{"--tensor-parallel-size", "-tp", "--max-model-len", "--dtype"},
		},
		{
			name:      "unknown backend",
			backend:   "unknown",
			expectNil: true,
		},
		{
			name:      "empty backend name",
			backend:   "",
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetAllowedFlags(tt.backend)

			if tt.expectNil {
				if result != nil {
					t.Errorf("GetAllowedFlags(%q) expected nil, got %v", tt.backend, result)
				}
				return
			}

			if result == nil {
				t.Fatalf("GetAllowedFlags(%q) returned nil, expected non-nil", tt.backend)
			}

			for _, flag := range tt.checkFlags {
				if !result[flag] {
					t.Errorf("GetAllowedFlags(%q) missing expected flag %q", tt.backend, flag)
				}
			}
		})
	}
}

func TestLlamaCppAllowedFlags(t *testing.T) {
	expectedFlags := []string{
		// Threading
		"-t", "--threads", "-tb", "--threads-batch",
		// Context
		"-c", "--ctx-size", "-n", "--n-predict", "-b", "--batch-size", "-ub", "--ubatch-size",
		// Sampling
		"--temp", "--temperature", "--top-k", "--top-p", "--min-p",
		"--repeat-last-n", "--repeat-penalty", "--presence-penalty", "--frequency-penalty",
		"--seed", "-s",
		// GPU
		"-ngl", "--gpu-layers", "--n-gpu-layers", "-sm", "--split-mode",
		"-ts", "--tensor-split", "-mg", "--main-gpu",
		"--mlock", "--mmap", "--no-mmap",
		// Server
		"-np", "--parallel", "--timeout", "-to",
		"-cb", "--cont-batching", "-fa", "--flash-attn", "--cache-prompt",
		// Mode
		"--embeddings", "--embedding", "--reranking",
		"--metrics", "--no-metrics", "--jinja",
		"-v", "--verbose", "--reasoning-budget",
		// RoPE
		"--rope-scaling", "--rope-scale", "--rope-freq-base", "--rope-freq-scale",
	}

	for _, flag := range expectedFlags {
		if !LlamaCppAllowedFlags[flag] {
			t.Errorf("LlamaCppAllowedFlags missing expected flag %q", flag)
		}
	}
}

func TestVLLMAllowedFlags(t *testing.T) {
	expectedFlags := []string{
		// Parallelism
		"--tensor-parallel-size", "-tp", "--pipeline-parallel-size", "-pp",
		// Model config
		"--max-model-len", "--max-num-batched-tokens", "--max-num-seqs",
		"--block-size", "--swap-space", "--seed",
		// Data types
		"--dtype", "--quantization", "-q", "--kv-cache-dtype",
		// Performance
		"--enforce-eager", "--enable-prefix-caching", "--enable-chunked-prefill",
		"--disable-custom-all-reduce", "--use-v2-block-manager",
		// Tokenizer
		"--tokenizer-mode", "--trust-remote-code", "--max-logprobs",
		// Misc
		"--revision", "--load-format", "--disable-log-stats", "--served-model-name",
	}

	for _, flag := range expectedFlags {
		if !VLLMAllowedFlags[flag] {
			t.Errorf("VLLMAllowedFlags missing expected flag %q", flag)
		}
	}
}

func TestDangerousFlagsNotAllowed(t *testing.T) {
	// Ensure dangerous flags are NOT in the allowlists
	dangerousFlags := []string{
		"--log-file",
		"--output-file",
		"--model-path",
		"--config-file",
		"--lora-path",
		"--grammar-file",
		"--prompt-file",
	}

	for _, flag := range dangerousFlags {
		if LlamaCppAllowedFlags[flag] {
			t.Errorf("Dangerous flag %q should not be in LlamaCppAllowedFlags", flag)
		}
		if VLLMAllowedFlags[flag] {
			t.Errorf("Dangerous flag %q should not be in VLLMAllowedFlags", flag)
		}
	}
}
