package sandbox

import (
	"os/exec"
)

// Sandbox encapsulates a single running sandboxed process.
type Sandbox interface {
	// Command returns the sandboxed process handle.
	Command() *exec.Cmd
	// Close closes the sandbox, terminating the process if it's still running.
	Close() error
}
