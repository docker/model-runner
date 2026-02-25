package envconfig

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/docker/model-runner/pkg/logging"
)

// Var returns an environment variable stripped of leading/trailing quotes and spaces.
func Var(key string) string {
	return strings.Trim(strings.TrimSpace(os.Getenv(key)), "\"'")
}

// String returns a lazy string accessor for the given environment variable.
func String(key string) func() string {
	return func() string {
		return Var(key)
	}
}

// BoolWithDefault returns a lazy bool accessor for the given environment variable,
// allowing a caller-specified default. If the variable is set but cannot be parsed
// as a bool, the defaultValue is returned.
func BoolWithDefault(key string) func(defaultValue bool) bool {
	return func(defaultValue bool) bool {
		if s := Var(key); s != "" {
			b, err := strconv.ParseBool(s)
			if err != nil {
				return defaultValue
			}
			return b
		}
		return defaultValue
	}
}

// Bool returns a lazy bool accessor that defaults to false when the variable is unset.
func Bool(key string) func() bool {
	withDefault := BoolWithDefault(key)
	return func() bool {
		return withDefault(false)
	}
}

// LogLevel reads LOG_LEVEL and returns the corresponding slog.Level.
func LogLevel() slog.Level {
	return logging.ParseLevel(Var("LOG_LEVEL"))
}

// AllowedOrigins returns a list of CORS-allowed origins. It reads DMR_ORIGINS
// and always appends default localhost/127.0.0.1/0.0.0.0 entries on http and
// https with wildcard ports.
func AllowedOrigins() (origins []string) {
	if s := Var("DMR_ORIGINS"); s != "" {
		for _, o := range strings.Split(s, ",") {
			if trimmed := strings.TrimSpace(o); trimmed != "" {
				origins = append(origins, trimmed)
			}
		}
	}

	for _, host := range []string{"localhost", "127.0.0.1", "0.0.0.0"} {
		origins = append(origins,
			fmt.Sprintf("http://%s", host),
			fmt.Sprintf("https://%s", host),
			fmt.Sprintf("http://%s", net.JoinHostPort(host, "*")),
			fmt.Sprintf("https://%s", net.JoinHostPort(host, "*")),
		)
	}

	return origins
}

// SocketPath returns the Unix socket path for the model runner.
// Configured via MODEL_RUNNER_SOCK; defaults to "model-runner.sock".
func SocketPath() string {
	if s := Var("MODEL_RUNNER_SOCK"); s != "" {
		return s
	}
	return "model-runner.sock"
}

// ModelsPath returns the directory where models are stored.
// Configured via MODELS_PATH; defaults to ~/.docker/models.
func ModelsPath() (string, error) {
	if s := Var("MODELS_PATH"); s != "" {
		return s, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".docker", "models"), nil
}

// TCPPort returns the optional TCP port for the model runner HTTP server.
// Configured via MODEL_RUNNER_PORT; empty string means use Unix socket.
func TCPPort() string {
	return Var("MODEL_RUNNER_PORT")
}

// LlamaServerPath returns the path to the llama.cpp server binary.
// Configured via LLAMA_SERVER_PATH; defaults to the Docker Desktop bundle location.
func LlamaServerPath() string {
	if s := Var("LLAMA_SERVER_PATH"); s != "" {
		return s
	}
	return "/Applications/Docker.app/Contents/Resources/model-runner/bin"
}

// LlamaArgs returns custom arguments to pass to the llama.cpp server.
// Configured via LLAMA_ARGS.
func LlamaArgs() string {
	return Var("LLAMA_ARGS")
}

// DisableServerUpdate is true when DISABLE_SERVER_UPDATE is set to a truthy value.
var DisableServerUpdate = Bool("DISABLE_SERVER_UPDATE")

// LlamaServerVersion returns a specific llama.cpp server version to pin.
// Configured via LLAMA_SERVER_VERSION; empty string means use the bundled version.
func LlamaServerVersion() string {
	return Var("LLAMA_SERVER_VERSION")
}

// VLLMServerPath returns the optional path to the vLLM server binary.
// Configured via VLLM_SERVER_PATH.
func VLLMServerPath() string {
	return Var("VLLM_SERVER_PATH")
}

// SGLangServerPath returns the optional path to the SGLang server binary.
// Configured via SGLANG_SERVER_PATH.
func SGLangServerPath() string {
	return Var("SGLANG_SERVER_PATH")
}

// MLXServerPath returns the optional path to the MLX server binary.
// Configured via MLX_SERVER_PATH.
func MLXServerPath() string {
	return Var("MLX_SERVER_PATH")
}

// DiffusersServerPath returns the optional path to the Diffusers server binary.
// Configured via DIFFUSERS_SERVER_PATH.
func DiffusersServerPath() string {
	return Var("DIFFUSERS_SERVER_PATH")
}

// VLLMMetalServerPath returns the optional path to the vLLM Metal server binary.
// Configured via VLLM_METAL_SERVER_PATH.
func VLLMMetalServerPath() string {
	return Var("VLLM_METAL_SERVER_PATH")
}

// DisableMetrics is true when DISABLE_METRICS is set to a truthy value (e.g. "1").
var DisableMetrics = Bool("DISABLE_METRICS")

// TLSEnabled is true when MODEL_RUNNER_TLS_ENABLED is set to a truthy value.
var TLSEnabled = Bool("MODEL_RUNNER_TLS_ENABLED")

// TLSPort returns the TLS listener port.
// Configured via MODEL_RUNNER_TLS_PORT; defaults to "12444".
func TLSPort() string {
	if s := Var("MODEL_RUNNER_TLS_PORT"); s != "" {
		return s
	}
	return "12444"
}

// TLSCert returns the path to the TLS certificate file.
// Configured via MODEL_RUNNER_TLS_CERT.
func TLSCert() string {
	return Var("MODEL_RUNNER_TLS_CERT")
}

// TLSKey returns the path to the TLS private key file.
// Configured via MODEL_RUNNER_TLS_KEY.
func TLSKey() string {
	return Var("MODEL_RUNNER_TLS_KEY")
}

// TLSAutoCert is true (default) unless MODEL_RUNNER_TLS_AUTO_CERT is set to a falsy value.
// Call as TLSAutoCert(true) to get the default-true behaviour.
var TLSAutoCert = BoolWithDefault("MODEL_RUNNER_TLS_AUTO_CERT")

// EnvVar describes a single environment variable with its current value
// and a human-readable description.
type EnvVar struct {
	Name        string
	Value       any
	Description string
}

// AsMap returns a map of all model-runner environment variables with their
// current values and descriptions. Useful for introspection and documentation.
func AsMap() map[string]EnvVar {
	modelsPath, _ := ModelsPath()
	return map[string]EnvVar{
		"MODEL_RUNNER_SOCK":          {"MODEL_RUNNER_SOCK", SocketPath(), "Unix socket path (default: model-runner.sock)"},
		"MODELS_PATH":                {"MODELS_PATH", modelsPath, "Directory for model storage (default: ~/.docker/models)"},
		"MODEL_RUNNER_PORT":          {"MODEL_RUNNER_PORT", TCPPort(), "TCP port; overrides Unix socket when set"},
		"LLAMA_SERVER_PATH":          {"LLAMA_SERVER_PATH", LlamaServerPath(), "Path to llama.cpp server binary"},
		"LLAMA_ARGS":                 {"LLAMA_ARGS", LlamaArgs(), "Extra arguments passed to the llama.cpp server"},
		"DISABLE_SERVER_UPDATE":      {"DISABLE_SERVER_UPDATE", DisableServerUpdate(), "Skip automatic llama.cpp server updates (any truthy value)"},
		"LLAMA_SERVER_VERSION":       {"LLAMA_SERVER_VERSION", LlamaServerVersion(), "Pin a specific llama.cpp server version"},
		"VLLM_SERVER_PATH":           {"VLLM_SERVER_PATH", VLLMServerPath(), "Path to vLLM server binary"},
		"SGLANG_SERVER_PATH":         {"SGLANG_SERVER_PATH", SGLangServerPath(), "Path to SGLang server binary"},
		"MLX_SERVER_PATH":            {"MLX_SERVER_PATH", MLXServerPath(), "Path to MLX server binary"},
		"DIFFUSERS_SERVER_PATH":      {"DIFFUSERS_SERVER_PATH", DiffusersServerPath(), "Path to Diffusers server binary"},
		"VLLM_METAL_SERVER_PATH":     {"VLLM_METAL_SERVER_PATH", VLLMMetalServerPath(), "Path to vLLM Metal server binary"},
		"DISABLE_METRICS":            {"DISABLE_METRICS", DisableMetrics(), "Disable Prometheus metrics endpoint (any truthy value, e.g. 1)"},
		"LOG_LEVEL":                  {"LOG_LEVEL", LogLevel(), "Log verbosity: debug, info, warn, error (default: info)"},
		"DMR_ORIGINS":                {"DMR_ORIGINS", AllowedOrigins(), "Comma-separated CORS allowed origins (defaults plus any env-provided origins)"},
		"MODEL_RUNNER_TLS_ENABLED":   {"MODEL_RUNNER_TLS_ENABLED", TLSEnabled(), "Enable TLS listener"},
		"MODEL_RUNNER_TLS_PORT":      {"MODEL_RUNNER_TLS_PORT", TLSPort(), "TLS listener port (default: 12444)"},
		"MODEL_RUNNER_TLS_CERT":      {"MODEL_RUNNER_TLS_CERT", TLSCert(), "Path to TLS certificate file"},
		"MODEL_RUNNER_TLS_KEY":       {"MODEL_RUNNER_TLS_KEY", TLSKey(), "Path to TLS private key file"},
		"MODEL_RUNNER_TLS_AUTO_CERT": {"MODEL_RUNNER_TLS_AUTO_CERT", TLSAutoCert(true), "Auto-generate TLS certificates (default: true)"},
	}
}
