package common

import (
	"errors"
	"io"
	"os"
	"strings"

	"github.com/docker/model-runner/pkg/internal/utils"
	"github.com/docker/model-runner/pkg/logging"
	"github.com/docker/model-runner/pkg/sandbox"
)

// SanitizedArgsLog logs command arguments with sanitization for safe logging
func SanitizedArgsLog(log logging.Logger, label string, args []string) {
	sanitizedArgs := make([]string, len(args))
	for i, arg := range args {
		sanitizedArgs[i] = utils.SanitizeForLog(arg)
	}
	log.Infof("%s: %v", label, sanitizedArgs)
}

// HandleSocketCleanup removes the socket file at the given path, ignoring if it doesn't exist
func HandleSocketCleanup(socket string) error {
	if err := os.RemoveAll(socket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// ProcessExitHandler handles the exit of a backend process and captures its output
func ProcessExitHandler(
	log logging.Logger,
	sandboxInstance sandbox.Sandbox,
	tailBuf io.ReadWriter,
	serverLogStream io.Closer,
	socket string,
) error {
	serverLogStream.Close()

	errOutput := new(strings.Builder)
	if _, err := io.Copy(errOutput, tailBuf); err != nil {
		log.Warnf("failed to read server output tail: %v", err)
	}

	cmdErr := sandboxInstance.Command().Wait()
	outputStr := errOutput.String()
	if len(outputStr) != 0 {
		return &BackendExitError{
			Err:    cmdErr,
			Output: outputStr,
		}
	} else {
		return cmdErr
	}
}

// BackendExitError represents an error when a backend process exits
type BackendExitError struct {
	Err    error
	Output string
}

func (e *BackendExitError) Error() string {
	if e.Output != "" {
		return e.Err.Error() + "\nwith output: " + e.Output
	}
	return e.Err.Error()
}

// SplitArgs splits a string into arguments, respecting quoted arguments
func SplitArgs(s string) []string {
	var args []string
	var currentArg strings.Builder
	inQuotes := false

	for _, r := range s {
		switch {
		case r == '"' || r == '\'':
			inQuotes = !inQuotes
		case r == ' ' && !inQuotes:
			if currentArg.Len() > 0 {
				args = append(args, currentArg.String())
				currentArg.Reset()
			}
		default:
			currentArg.WriteRune(r)
		}
	}

	if currentArg.Len() > 0 {
		args = append(args, currentArg.String())
	}

	return args
}