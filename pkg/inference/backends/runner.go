package backends

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/docker/model-runner/pkg/internal/utils"
	"github.com/docker/model-runner/pkg/sandbox"
	"github.com/docker/model-runner/pkg/tailbuffer"
)

// ErrorTransformer is a function that transforms raw error output
// into a more user-friendly message. Backends can provide their own
// implementation to customize error presentation.
type ErrorTransformer func(output string) string

// RunnerConfig holds configuration for a backend runner
type RunnerConfig struct {
	// BackendName is the display name of the backend (e.g., "llama.cpp", "vLLM")
	BackendName string
	// Socket is the unix socket path
	Socket string
	// BinaryPath is the path to the backend binary
	BinaryPath string
	// SandboxPath is the sandbox directory path
	SandboxPath string
	// SandboxConfig is the sandbox configuration string
	SandboxConfig string
	// Args are the command line arguments
	Args []string
	// Logger provides logging functionality
	Logger Logger
	// ServerLogWriter provides a writer for server logs
	ServerLogWriter io.WriteCloser
	// ErrorTransformer is an optional function to transform error output
	// into a more user-friendly message. If nil, the raw output is used.
	ErrorTransformer ErrorTransformer
}

// Logger interface for backend logging
type Logger interface {
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Warnln(args ...interface{})
}

// RunBackend runs a backend process with common error handling and logging.
// It handles:
// - Socket cleanup
// - Argument sanitization for logging
// - Process lifecycle management
// - Error channel handling
// - Context cancellation
func RunBackend(ctx context.Context, config RunnerConfig) error {
	// Remove old socket file
	if err := os.RemoveAll(config.Socket); err != nil && !errors.Is(err, fs.ErrNotExist) {
		config.Logger.Warnf("failed to remove socket file %s: %v\n", config.Socket, err)
		config.Logger.Warnln(config.BackendName + " may not be able to start")
	}

	// Sanitize args for safe logging
	sanitizedArgs := make([]string, len(config.Args))
	for i, arg := range config.Args {
		sanitizedArgs[i] = utils.SanitizeForLog(arg, 0)
	}
	config.Logger.Infof("%s args: %v", config.BackendName, sanitizedArgs)

	// Create tail buffer for error output
	tailBuf := tailbuffer.NewTailBuffer(1024)
	out := io.MultiWriter(config.ServerLogWriter, tailBuf)

	// Create sandbox with process cancellation
	backendSandbox, err := sandbox.Create(
		ctx,
		config.SandboxConfig,
		func(command *exec.Cmd) {
			command.Cancel = func() error {
				if runtime.GOOS == "windows" {
					return command.Process.Kill()
				}
				return command.Process.Signal(os.Interrupt)
			}
			command.Stdout = config.ServerLogWriter
			command.Stderr = out
		},
		config.SandboxPath,
		config.BinaryPath,
		config.Args...,
	)
	if err != nil {
		return fmt.Errorf("unable to start %s: %w", config.BackendName, err)
	}
	defer backendSandbox.Close()

	// Handle backend process errors
	backendErrors := make(chan error, 1)
	go func() {
		backendErr := backendSandbox.Command().Wait()
		config.ServerLogWriter.Close()

		errOutput := new(strings.Builder)
		if _, err := io.Copy(errOutput, tailBuf); err != nil {
			config.Logger.Warnf("failed to read server output tail: %v", err)
		}

		if errOutput.String() != "" {
			errorMsg := errOutput.String()
			// Apply error transformer if provided
			if config.ErrorTransformer != nil {
				errorMsg = config.ErrorTransformer(errorMsg)
			}
			backendErr = fmt.Errorf("%s failed: %s", config.BackendName, errorMsg)
		} else {
			backendErr = fmt.Errorf("%s exit status: %w", config.BackendName, backendErr)
		}

		backendErrors <- backendErr
		close(backendErrors)
		if err := os.Remove(config.Socket); err != nil && !errors.Is(err, fs.ErrNotExist) {
			config.Logger.Warnf("failed to remove socket file %s on exit: %v\n", config.Socket, err)
		}
	}()
	defer func() {
		<-backendErrors
	}()

	// Wait for context cancellation or backend errors
	select {
	case <-ctx.Done():
		return nil
	case backendErr := <-backendErrors:
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		return fmt.Errorf("%s terminated unexpectedly: %w", config.BackendName, backendErr)
	}
}
