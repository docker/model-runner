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
			checkFlags: []string{"--threads", "-t", "--ctx-size", "-ngl", "--verbose", "-v", "--cache-type-k", "--cache-type-v"},
		},
		{
			name:       "vllm backend",
			backend:    "vllm",
			expectNil:  false,
			checkFlags: []string{"--tensor-parallel-size", "-tp", "--max-model-len", "--dtype", "--gpu-memory-utilization"},
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

func TestLlamaCppAllowedFlags_Categories(t *testing.T) {
	// Test that key flags from each category are present
	categories := map[string][]string{
		"threading": {"-t", "--threads", "-tb", "--threads-batch", "-C", "--cpu-mask", "--prio"},
		"context":   {"-c", "--ctx-size", "-n", "--n-predict", "--keep"},
		"batching":  {"-b", "--batch-size", "-ub", "--ubatch-size", "-fa", "--flash-attn"},
		"sampling": {
			"--samplers", "-s", "--seed", "--temp", "--temperature",
			"--top-k", "--top-p", "--min-p", "--typical",
			"--repeat-last-n", "--repeat-penalty",
			"--presence-penalty", "--frequency-penalty",
			"--mirostat", "--mirostat-lr", "--mirostat-ent",
			"--dynatemp-range", "--dynatemp-exp",
		},
		"gpu": {
			"-ngl", "--gpu-layers", "--n-gpu-layers",
			"-sm", "--split-mode", "-ts", "--tensor-split",
			"-mg", "--main-gpu", "-dev", "--device",
		},
		"memory": {
			"--mlock", "--mmap", "--no-mmap",
			"-ctk", "--cache-type-k", "-ctv", "--cache-type-v",
			"-kvo", "--kv-offload", "-nkvo", "--no-kv-offload",
			"-cram", "--cache-ram",
		},
		"rope": {
			"--rope-scaling", "--rope-scale",
			"--rope-freq-base", "--rope-freq-scale",
			"--yarn-orig-ctx", "--yarn-ext-factor",
		},
		"server": {
			"-np", "--parallel", "-to", "--timeout",
			"-cb", "--cont-batching", "--cache-prompt",
			"--threads-http", "--warmup", "--no-warmup",
		},
		"mode": {
			"--embeddings", "--embedding", "--reranking", "--rerank",
			"--metrics", "--no-metrics", "--jinja", "--no-jinja",
		},
		"speculative": {
			"--draft", "--draft-max", "--draft-min",
			"-cd", "--ctx-size-draft",
			"-ngld", "--gpu-layers-draft",
		},
	}

	for category, flags := range categories {
		t.Run(category, func(t *testing.T) {
			for _, flag := range flags {
				if !LlamaCppAllowedFlags[flag] {
					t.Errorf("LlamaCppAllowedFlags missing %s flag %q", category, flag)
				}
			}
		})
	}
}

func TestVLLMAllowedFlags_Categories(t *testing.T) {
	categories := map[string][]string{
		"parallelism": {"--tensor-parallel-size", "-tp", "--pipeline-parallel-size", "-pp"},
		"model":       {"--max-model-len", "--max-num-batched-tokens", "--max-num-seqs", "--block-size", "--swap-space", "--seed"},
		"dtype":       {"--dtype", "--quantization", "-q", "--kv-cache-dtype"},
		"performance": {"--enforce-eager", "--enable-prefix-caching", "--enable-chunked-prefill"},
		"tokenizer":   {"--tokenizer-mode", "--trust-remote-code", "--max-logprobs"},
		"misc":        {"--revision", "--load-format", "--disable-log-stats", "--served-model-name", "--gpu-memory-utilization"},
	}

	for category, flags := range categories {
		t.Run(category, func(t *testing.T) {
			for _, flag := range flags {
				if !VLLMAllowedFlags[flag] {
					t.Errorf("VLLMAllowedFlags missing %s flag %q", category, flag)
				}
			}
		})
	}
}

func TestDangerousFlagsNotAllowed(t *testing.T) {
	// Ensure dangerous flags involving file paths are NOT in the allowlists
	dangerousFlags := []string{
		// File path flags
		"--log-file",
		"--output-file",
		"--model-path",
		"--config-file",
		"--lora-path",
		"--grammar-file",
		"--prompt-file",
		// llama.cpp specific path flags
		"--slot-save-path",
		"-mm", "--mmproj",
		"-mmu", "--mmproj-url",
		"-jf", "--json-schema-file",
		"--chat-template-file",
		"--path",
		"--webui-config-file",
		"--api-key-file",
		"--ssl-key-file",
		"--ssl-cert-file",
		"--models-dir",
		"--models-preset",
		"-md", "--model-draft",
		"--lora",
		"--lora-scaled",
		"--control-vector",
		"--control-vector-scaled",
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

func TestIssue515Flags(t *testing.T) {
	// Verify all flags from GitHub issue #515 are allowed
	issue515Flags := []string{
		"--n-gpu-layers",
		"--no-mmap",
		"--flash-attn",
		"--jinja",
		"--top-p",
		"--top-k",
		"--temp",
		"--min-p",
		"--presence-penalty",
		"--cache-type-k",
		"--cache-type-v",
		"--n-predict",
		"--threads",
	}

	for _, flag := range issue515Flags {
		if !LlamaCppAllowedFlags[flag] {
			t.Errorf("Flag %q from issue #515 should be in LlamaCppAllowedFlags", flag)
		}
	}
}
