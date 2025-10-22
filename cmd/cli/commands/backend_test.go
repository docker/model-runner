package commands

import (
	"os"
	"testing"
)

func TestResolveServerURL(t *testing.T) {
	tests := []struct {
		name        string
		host        string
		customURL   string
		urlAlias    string
		port        int
		expectURL   string
		expectOAI   bool
		wantErr     bool
		setupEnv    func()
		cleanupEnv  func()
	}{
		{
			name:      "no flags specified",
			expectURL: "",
			expectOAI: false,
			wantErr:   false,
		},
		{
			name:      "host and port specified",
			host:      "192.168.1.1",
			port:      8080,
			expectURL: "http://192.168.1.1:8080",
			expectOAI: false,
			wantErr:   false,
		},
		{
			name:      "only host specified",
			host:      "192.168.1.1",
			expectURL: "http://192.168.1.1:12434",
			expectOAI: false,
			wantErr:   false,
		},
		{
			name:      "only port specified",
			port:      8080,
			expectURL: "http://127.0.0.1:8080",
			expectOAI: false,
			wantErr:   false,
		},
		{
			name:      "llamacpp url-alias specified",
			urlAlias:  "llamacpp",
			expectURL: "http://127.0.0.1:8080/v1",
			expectOAI: true,
			wantErr:   false,
		},
		{
			name:      "ollama url-alias specified",
			urlAlias:  "ollama",
			expectURL: "http://127.0.0.1:11434/v1",
			expectOAI: true,
			wantErr:   false,
		},
		{
			name:     "openrouter url-alias without API key",
			urlAlias: "openrouter",
			wantErr:  true,
		},
		{
			name:      "openrouter url-alias with API key",
			urlAlias:  "openrouter",
			expectURL: "https://openrouter.ai/api/v1",
			expectOAI: true,
			wantErr:   false,
			setupEnv: func() {
				os.Setenv("OPENAI_API_KEY", "test-key")
			},
			cleanupEnv: func() {
				os.Unsetenv("OPENAI_API_KEY")
			},
		},
		{
			name:      "custom URL specified",
			customURL: "http://custom.server.com:9000/v1",
			expectURL: "http://custom.server.com:9000/v1",
			expectOAI: true,
			wantErr:   false,
		},
		{
			name:      "multiple preset flags (url + url-alias)",
			customURL: "http://test.com/v1",
			urlAlias:  "ollama",
			wantErr:   true,
		},
		{
			name:     "host/port with url-alias",
			host:     "192.168.1.1",
			urlAlias: "llamacpp",
			wantErr:  true,
		},
		{
			name:      "host/port with url",
			host:      "192.168.1.1",
			customURL: "http://test.com/v1",
			wantErr:   true,
		},
		{
			name:     "invalid url-alias",
			urlAlias: "invalid",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupEnv != nil {
				tt.setupEnv()
			}
			if tt.cleanupEnv != nil {
				defer tt.cleanupEnv()
			}

			url, useOAI, apiKey, err := resolveServerURL(tt.host, tt.customURL, tt.urlAlias, tt.port)

			if (err != nil) != tt.wantErr {
				t.Errorf("resolveServerURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
			}

			if url != tt.expectURL {
				t.Errorf("resolveServerURL() url = %v, want %v", url, tt.expectURL)
			}

			if useOAI != tt.expectOAI {
				t.Errorf("resolveServerURL() useOAI = %v, want %v", useOAI, tt.expectOAI)
			}

			// For openrouter, check that API key is returned
			if tt.urlAlias == "openrouter" && !tt.wantErr {
				if apiKey == "" {
					t.Errorf("resolveServerURL() expected API key for openrouter, got empty string")
				}
			}
		})
	}
}

func TestValidateBackend(t *testing.T) {
	tests := []struct {
		name    string
		backend string
		wantErr bool
	}{
		{
			name:    "valid backend llama.cpp",
			backend: "llama.cpp",
			wantErr: false,
		},
		{
			name:    "valid backend openai",
			backend: "openai",
			wantErr: false,
		},
		{
			name:    "invalid backend",
			backend: "invalid",
			wantErr: true,
		},
		{
			name:    "empty backend",
			backend: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBackend(tt.backend)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateBackend() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEnsureAPIKey(t *testing.T) {
	tests := []struct {
		name       string
		backend    string
		setupEnv   func()
		cleanupEnv func()
		wantErr    bool
		wantKey    string
	}{
		{
			name:    "non-openai backend",
			backend: "llama.cpp",
			wantErr: false,
			wantKey: "",
		},
		{
			name:    "openai backend without key",
			backend: "openai",
			wantErr: true,
		},
		{
			name:    "openai backend with key",
			backend: "openai",
			setupEnv: func() {
				os.Setenv("OPENAI_API_KEY", "test-key")
			},
			cleanupEnv: func() {
				os.Unsetenv("OPENAI_API_KEY")
			},
			wantErr: false,
			wantKey: "test-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupEnv != nil {
				tt.setupEnv()
			}
			if tt.cleanupEnv != nil {
				defer tt.cleanupEnv()
			}

			key, err := ensureAPIKey(tt.backend)
			if (err != nil) != tt.wantErr {
				t.Errorf("ensureAPIKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if key != tt.wantKey {
				t.Errorf("ensureAPIKey() key = %v, want %v", key, tt.wantKey)
			}
		})
	}
}
