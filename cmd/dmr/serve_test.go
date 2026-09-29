package main

import (
	"os"
	"testing"
)

func TestApplyServeEnv(t *testing.T) {
	tests := []struct {
		name       string
		opts       serveOptions
		presetEnv  map[string]string
		wantPort   string
		wantSocket string
	}{
		{
			name:     "defaults to standalone TCP port when nothing is set",
			opts:     serveOptions{},
			wantPort: defaultPort,
		},
		{
			name:     "explicit port flag wins",
			opts:     serveOptions{port: "9999"},
			wantPort: "9999",
		},
		{
			name:       "explicit socket flag wins",
			opts:       serveOptions{socket: "/tmp/dmr.sock"},
			wantSocket: "/tmp/dmr.sock",
		},
		{
			name:       "explicit socket flag clears an inherited MODEL_RUNNER_PORT",
			opts:       serveOptions{socket: "/tmp/dmr.sock"},
			presetEnv:  map[string]string{"MODEL_RUNNER_PORT": "12434"},
			wantSocket: "/tmp/dmr.sock",
		},
		{
			name:      "explicit port flag clears an inherited MODEL_RUNNER_SOCK",
			opts:      serveOptions{port: "9999"},
			presetEnv: map[string]string{"MODEL_RUNNER_SOCK": "/tmp/other.sock"},
			wantPort:  "9999",
		},
		{
			name:       "pre-existing MODEL_RUNNER_SOCK is left alone without flags",
			opts:       serveOptions{},
			presetEnv:  map[string]string{"MODEL_RUNNER_SOCK": "/tmp/other.sock"},
			wantSocket: "/tmp/other.sock",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// t.Setenv both isolates these process-global variables between
			// subtests and restores their prior value afterwards; an empty
			// string is equivalent to "unset" for every reader involved here
			// (os.Getenv, envconfig.Var).
			for _, key := range []string{"MODEL_RUNNER_PORT", "MODEL_RUNNER_SOCK", "MODELS_PATH"} {
				t.Setenv(key, "")
			}
			for k, v := range tt.presetEnv {
				t.Setenv(k, v)
			}

			if err := applyServeEnv(tt.opts); err != nil {
				t.Fatalf("applyServeEnv() error = %v", err)
			}

			if got := os.Getenv("MODEL_RUNNER_PORT"); got != tt.wantPort {
				t.Errorf("MODEL_RUNNER_PORT = %q, want %q", got, tt.wantPort)
			}
			if got := os.Getenv("MODEL_RUNNER_SOCK"); got != tt.wantSocket {
				t.Errorf("MODEL_RUNNER_SOCK = %q, want %q", got, tt.wantSocket)
			}
		})
	}
}
