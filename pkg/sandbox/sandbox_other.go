//go:build !darwin && !windows

package sandbox

import (
	"context"
	"fmt"
	"os/exec"
)

// ConfigurationLlamaCpp is the sandbox configuration for llama.cpp processes.
const ConfigurationLlamaCpp = ``

// sandbox is the non-Darwin POSIX sandbox implementation.
type sandbox struct {
	// cancel cancels the context associated with the process.
	cancel context.CancelFunc
	// command is the sandboxed process handle.
	command *exec.Cmd
}

// Command implements Sandbox.Command.
func (s *sandbox) Command() *exec.Cmd {
	return s.command
}

// Command implements Sandbox.Close.
func (s *sandbox) Close() error {
	s.cancel()
	return nil
}

// Create creates a sandbox containing a single process that has been started.
// The ctx, name, and arg arguments correspond to their counterparts in
// os/exec.CommandContext. The configuration argument specifies the sandbox
// configuration, for which a pre-defined value should be used. The modifier
// function allows for an optional callback (which may be nil) to configure the
// command before it is started.
func Create(ctx context.Context, configuration string, modifier func(*exec.Cmd), updatedBinPath, name string, arg ...string) (Sandbox, error) {
	// Create a subcontext we can use to regulate the process lifetime.
	ctx, cancel := context.WithCancel(ctx)

	// Create and configure the command.
	command := exec.CommandContext(ctx, name, arg...)
	if modifier != nil {
		modifier(command)
	}

	// Start the process.
	if err := command.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("unable to start process: %w", err)
	}
	return &sandbox{
		cancel:  cancel,
		command: command,
	}, nil
}
