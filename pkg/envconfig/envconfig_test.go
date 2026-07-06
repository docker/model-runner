package envconfig_test

import (
	"runtime"
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

	t.Run("default depends on platform", func(t *testing.T) {
		t.Setenv("LLAMA_SERVER_PATH", "")
		got := envconfig.LlamaServerPath()
		if runtime.GOOS == "darwin" {
			want := "/Applications/Docker.app/Contents/Resources/model-runner/bin"
			if got != want {
				t.Fatalf("LlamaServerPath() = %q, want %q", got, want)
			}
		} else if got != "" {
			t.Fatalf("LlamaServerPath() = %q, want empty string on %s", got, runtime.GOOS)
		}
	})
}
