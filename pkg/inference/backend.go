package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// BackendMode encodes the mode in which a backend should operate.
type BackendMode uint8

const (
	// BackendModeCompletion indicates that the backend should run in chat
	// completion mode.
	BackendModeCompletion BackendMode = iota
	// BackendModeEmbedding indicates that the backend should run in embedding
	// mode.
	BackendModeEmbedding
	BackendModeReranking
	// BackendModeImageGeneration indicates that the backend should run in
	// image generation mode.
	BackendModeImageGeneration
)

// Backend status constants for standardized status reporting.
// Backends should use these prefixes when reporting their status.
const (
	// StatusRunning indicates the backend is operational and ready.
	// Format: "Running: <details>" (e.g., "Running: vllm v0.1.0")
	StatusRunning = "Running"

	// StatusError indicates the backend encountered an error.
	// Format: "Error: <details>" (e.g., "Error: installation failed")
	StatusError = "Error"

	// StatusNotInstalled indicates the backend is not installed.
	// Format: "Not Installed: <details>" or just "Not Installed"
	StatusNotInstalled = "Not Installed"

	// StatusInstalling indicates the backend is currently being installed.
	// Format: "Installing: <details>" or just "Installing"
	StatusInstalling = "Installing"
)

// Common status detail messages for consistent reporting across backends.
const (
	DetailBinaryNotFound      = "binary not found"
	DetailPackageNotInstalled = "package not installed"
	DetailImportFailed        = "import failed"
	DetailVersionUnknown      = "version unknown"
	DetailPythonNotFound      = "Python not found"
	DetailOnlyLinux           = "only supported on Linux"
	DetailOnlyAppleSilicon    = "only supported on Apple Silicon"
	DetailDownloading         = "downloading"
	DetailCheckingForUpdates  = "checking for updates"
)

// FormatStatus formats a backend status with optional details.
// If details is empty, returns just the status type.
// Otherwise, returns "Status: details".
func FormatStatus(statusType, details string) string {
	if details == "" {
		return statusType
	}
	return statusType + ": " + details
}

// ParseStatus splits a formatted status string into type and details.
// Returns the status type and details separately.
func ParseStatus(status string) (statusType, details string) {
	if status == "" {
		return StatusNotInstalled, ""
	}

	for _, prefix := range []string{StatusRunning, StatusError, StatusNotInstalled, StatusInstalling} {
		if status == prefix {
			return prefix, ""
		}
		if details, found := strings.CutPrefix(status, prefix+": "); found {
			return prefix, details
		}
	}

	return StatusError, status
}

// FormatRunning formats a running status with version/details.
// Example: FormatRunning("vllm 0.1.0") -> "Running: vllm 0.1.0"
func FormatRunning(details string) string {
	return FormatStatus(StatusRunning, details)
}

// FormatError formats an error status with error message.
// Example: FormatError("installation failed") -> "Error: installation failed"
func FormatError(details string) string {
	return FormatStatus(StatusError, details)
}

// FormatNotInstalled formats a not installed status with optional details.
// Example: FormatNotInstalled("package not found") -> "Not Installed: package not found"
// Example: FormatNotInstalled("") -> "Not Installed"
func FormatNotInstalled(details string) string {
	return FormatStatus(StatusNotInstalled, details)
}

// FormatInstalling formats an installing status with optional details.
// Example: FormatInstalling("downloading") -> "Installing: downloading"
func FormatInstalling(details string) string {
	return FormatStatus(StatusInstalling, details)
}

type ErrGGUFParse struct {
	Err error
}

func (e *ErrGGUFParse) Error() string {
	return "failed to parse GGUF: " + e.Err.Error()
}

// String implements Stringer.String for BackendMode.
func (m BackendMode) String() string {
	switch m {
	case BackendModeCompletion:
		return "completion"
	case BackendModeEmbedding:
		return "embedding"
	case BackendModeReranking:
		return "reranking"
	case BackendModeImageGeneration:
		return "image-generation"
	default:
		return "unknown"
	}
}

// MarshalJSON implements json.Marshaler for BackendMode.
func (m BackendMode) MarshalJSON() ([]byte, error) {
	return []byte(`"` + m.String() + `"`), nil
}

// UnmarshalJSON implements json.Unmarshaler for BackendMode.
func (m *BackendMode) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	mode, ok := ParseBackendMode(s)
	if !ok {
		return fmt.Errorf("unknown backend mode: %q", s)
	}
	*m = mode
	return nil
}

// ParseBackendMode converts a string mode to BackendMode.
// It returns the parsed mode and a boolean indicating if the mode was known.
// For unknown modes, it returns BackendModeCompletion and false.
func ParseBackendMode(mode string) (BackendMode, bool) {
	switch mode {
	case "completion":
		return BackendModeCompletion, true
	case "embedding":
		return BackendModeEmbedding, true
	case "reranking":
		return BackendModeReranking, true
	case "image-generation":
		return BackendModeImageGeneration, true
	default:
		return BackendModeCompletion, false
	}
}

type SpeculativeDecodingConfig struct {
	DraftModel        string  `json:"draft_model,omitempty"`
	NumTokens         int     `json:"num_tokens,omitempty"`
	MinAcceptanceRate float64 `json:"min_acceptance_rate,omitempty"`
}

// VLLMConfig contains vLLM-specific configuration options.
type VLLMConfig struct {
	// HFOverrides contains HuggingFace model configuration overrides.
	// This maps to vLLM's --hf-overrides flag which accepts a JSON dictionary.
	HFOverrides HFOverrides `json:"hf-overrides,omitempty"`
	// GPUMemoryUtilization sets the fraction of GPU memory to be used for the model executor.
	// Must be between 0.0 and 1.0. If not specified, vLLM uses its default value of 0.9.
	// This maps to vLLM's --gpu-memory-utilization flag.
	GPUMemoryUtilization *float64 `json:"gpu-memory-utilization,omitempty"`
}

// LlamaCppConfig contains llama.cpp-specific configuration options.
type LlamaCppConfig struct {
	// ReasoningBudget sets the reasoning budget for reasoning models.
	// Maps to llama.cpp's --reasoning-budget flag.
	ReasoningBudget *int32 `json:"reasoning-budget,omitempty"`
}

// KeepAlive is a duration controlling how long a model stays loaded in memory.
// JSON representation uses Go duration strings (e.g. "5m", "1h") plus the
// special value "-1" (never unload). A nil *KeepAlive means use the default
// (5 minutes).
type KeepAlive time.Duration

const (
	KeepAliveDefault   = KeepAlive(5 * time.Minute)
	KeepAliveImmediate = KeepAlive(0)
	KeepAliveForever   = KeepAlive(-1)
)

func (d KeepAlive) Duration() time.Duration {
	return time.Duration(d)
}

func (d KeepAlive) MarshalJSON() ([]byte, error) {
	if d == KeepAliveForever {
		return json.Marshal("-1")
	}
	return json.Marshal(time.Duration(d).String())
}

func (d *KeepAlive) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := ParseKeepAlive(s)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// ParseKeepAlive converts a keep_alive string to a KeepAlive value.
// Accepts:
//   - Go duration strings: "5m", "1h", "30s"
//   - "0" to unload immediately
//   - Any negative value ("-1", "-1m") to keep loaded forever
func ParseKeepAlive(s string) (KeepAlive, error) {
	if s == "0" {
		return KeepAliveImmediate, nil
	}
	if s == "-1" {
		return KeepAliveForever, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid keep_alive duration %q: %w", s, err)
	}
	if d < 0 {
		return KeepAliveForever, nil
	}
	return KeepAlive(d), nil
}

type BackendConfiguration struct {
	// Shared configuration across all backends
	ContextSize  *int32                     `json:"context-size,omitempty"`
	RuntimeFlags []string                   `json:"runtime-flags,omitempty"`
	Speculative  *SpeculativeDecodingConfig `json:"speculative,omitempty"`
	KeepAlive    *KeepAlive                 `json:"keep_alive,omitempty"`

	// Backend-specific configuration
	VLLM     *VLLMConfig     `json:"vllm,omitempty"`
	LlamaCpp *LlamaCppConfig `json:"llamacpp,omitempty"`
}

type RequiredMemory struct {
	RAM  uint64
	VRAM uint64 // TODO(p1-0tr): for now assume we are working with single GPU set-ups
}

// Backend is the interface implemented by inference engine backends. Backend
// implementations need not be safe for concurrent invocation of the following
// methods, though their underlying server implementations do need to support
// concurrent API requests.
type Backend interface {
	// Name returns the backend name. It must be all lowercase and usable as a
	// path component in an HTTP request path and a Unix domain socket path. It
	// should also be suitable for presenting to users (at least in logs). The
	// package providing the backend implementation should also expose a
	// constant called Name which matches the value returned by this method.
	Name() string
	// UsesExternalModelManagement should return true if the backend uses an
	// external model management system and false if the backend uses the shared
	// model manager.
	UsesExternalModelManagement() bool
	// UsesTCP returns true if the backend uses TCP for communication instead
	// of Unix sockets. When true, the scheduler will create a TCP transport
	// and pass a "host:port" address to Run instead of a Unix socket path.
	UsesTCP() bool
	// Install ensures that the backend is installed. It should return a nil
	// error if installation succeeds or if the backend is already installed.
	// The provided HTTP client should be used for any HTTP operations.
	Install(ctx context.Context, httpClient *http.Client) error
	// Run runs an OpenAI API web server on the specified Unix domain socket
	// for the specified model using the backend. It should start any
	// process(es) necessary for the backend to function for the model. It
	// should not return until either the process(es) fail or the provided
	// context is cancelled. By the time Run returns, any process(es) it has
	// spawned must terminate.
	//
	// Backend implementations should be "one-shot" (i.e. returning from Run
	// after the failure of an underlying process). Backends should not attempt
	// to perform restarts on failure. Backends should only return a nil error
	// in the case of context cancellation, otherwise they should return the
	// error that caused them to fail.
	//
	// Run will be provided with the path to a Unix domain socket on which the
	// backend should listen for incoming OpenAI API requests and a model name
	// to be loaded. Backends should not load multiple models at once and should
	// instead load only the specified model. Backends should still respond to
	// OpenAI API requests for other models with a 421 error code.
	Run(ctx context.Context, socket, model string, modelRef string, mode BackendMode, config *BackendConfiguration) error
	// Status returns a description of the backend's state.
	Status() string
	// GetDiskUsage returns the disk usage of the backend.
	GetDiskUsage() (int64, error)
}
