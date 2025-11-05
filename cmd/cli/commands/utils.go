package commands

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/docker/cli/cli-plugins/hooks"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/cmd/cli/pkg/standalone"
	"github.com/docker/model-runner/pkg/inference/backends/vllm"
	"github.com/moby/term"
	"github.com/spf13/cobra"
)

const (
	enableViaCLI = "Enable Docker Model Runner via the CLI → docker desktop enable model-runner"
	enableViaGUI = "Enable Docker Model Runner via the GUI → Go to Settings->AI->Enable Docker Model Runner"
	enableVLLM   = "It looks like you're trying to use a model for vLLM → docker model install-runner --vllm"
)

var notRunningErr = fmt.Errorf("Docker Model Runner is not running. Please start it and try again.\n")

func handleClientError(err error, message string) error {
	if errors.Is(err, desktop.ErrServiceUnavailable) {
		err = notRunningErr
		var buf bytes.Buffer
		hooks.PrintNextSteps(&buf, []string{enableViaCLI, enableViaGUI})
		return fmt.Errorf("%w\n%s", err, strings.TrimRight(buf.String(), "\n"))
	} else if strings.Contains(err.Error(), vllm.StatusNotFound.Error()) {
		// Handle `run` error.
		var buf bytes.Buffer
		hooks.PrintNextSteps(&buf, []string{enableVLLM})
		return fmt.Errorf("%w\n%s", err, strings.TrimRight(buf.String(), "\n"))
	}
	return fmt.Errorf("%s: %w", message, err)
}

// commandPrinter wraps a cobra.Command to implement standalone.StatusPrinter
type commandPrinter struct {
	cmd *cobra.Command
}

// Printf implements StatusPrinter.Printf by delegating to cobra.Command.Printf
func (cp *commandPrinter) Printf(format string, args ...any) {
	cp.cmd.Printf(format, args...)
}

// Println implements StatusPrinter.Println by delegating to cobra.Command.Println
func (cp *commandPrinter) Println(args ...any) {
	cp.cmd.Println(args...)
}

// PrintErrf implements StatusPrinter.PrintErrf by delegating to cobra.Command.PrintErrf
func (cp *commandPrinter) PrintErrf(format string, args ...any) {
	cp.cmd.PrintErrf(format, args...)
}

// Write implements StatusPrinter.Write by delegating to cobra.Command's output writer
func (cp *commandPrinter) Write(p []byte) (n int, err error) {
	return cp.cmd.OutOrStdout().Write(p)
}

// GetFdInfo returns the file descriptor and terminal status of the command's output
func (cp *commandPrinter) GetFdInfo() (fd uintptr, isTerminal bool) {
	out := cp.cmd.OutOrStdout()

	if file, ok := out.(*os.File); ok {
		return term.GetFdInfo(file)
	}

	// For progress display, we care about whether stdout is a terminal
	// Even if cobra wraps the output, checking os.Stdout directly is appropriate
	// because that's where the visual progress bars should be displayed
	return term.GetFdInfo(os.Stdout)
}

// asPrinter wraps a cobra.Command to implement standalone.StatusPrinter
func asPrinter(cmd *cobra.Command) standalone.StatusPrinter {
	return &commandPrinter{cmd: cmd}
}
