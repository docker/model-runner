package commands

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/docker/cli/cli-plugins/hooks"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/pkg/errors"
)

const (
	enableViaCLI = "Enable Docker Model Runner via the CLI → docker desktop enable model-runner"
	enableViaGUI = "Enable Docker Model Runner via the GUI → Go to Settings->AI->Enable Docker Model Runner"
)

var notRunningErr = fmt.Errorf("Docker Model Runner is not running. Please start it and try again.\n")

// isOllamaModel checks if a model name is from ollama.com
func isOllamaModel(model string) bool {
	return strings.HasPrefix(model, "ollama.com/")
}

func handleClientError(err error, message string) error {
	if errors.Is(err, desktop.ErrServiceUnavailable) {
		return notRunningErr
	}
	return errors.Wrap(err, message)
}

func handleNotRunningError(err error) error {
	if errors.Is(err, notRunningErr) {
		var buf bytes.Buffer
		hooks.PrintNextSteps(&buf, []string{enableViaCLI, enableViaGUI})
		return fmt.Errorf("%w\n%s", err, strings.TrimRight(buf.String(), "\n"))
	}
	return err
}
