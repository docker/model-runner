package envconfig_test

import (
	"testing"

	"github.com/docker/model-runner/pkg/envconfig"
)

func TestLlamaServerPath(t *testing.T) {
	t.Run("explicit override wins", func(t *testing.T) {
		t.Setenv("LLAMA_SERVER_PATH", "/custom/path")
		if got := envconfig.LlamaServerPath(); got != "/custom/path" {
			t.Fatalf("LlamaServerPath() = %q, want %q", got, "/custom/path")
		}
	})

	t.Run("empty when unset", func(t *testing.T) {
		t.Setenv("LLAMA_SERVER_PATH", "")
		// When unset there is no meaningful default here: the llama.cpp backend
		// falls back to a writable per-user directory (macOS/Windows) or the
		// image sets LLAMA_SERVER_PATH explicitly (Linux).
		if got := envconfig.LlamaServerPath(); got != "" {
			t.Fatalf("LlamaServerPath() = %q, want empty string", got)
		}
	})
}
