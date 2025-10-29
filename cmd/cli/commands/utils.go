package commands

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/docker/cli/cli-plugins/hooks"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/pkg/inference/backends/vllm"
)

const (
	defaultOrg = "ai"
	defaultTag = "latest"
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
	return errors.Join(err, errors.New(message))
}

// stripDefaultsFromModelName removes the default "ai/" prefix and ":latest" tag for display.
// Examples:
//   - "ai/gemma3:latest" -> "gemma3"
//   - "ai/gemma3:v1" -> "ai/gemma3:v1"
//   - "myorg/gemma3:latest" -> "myorg/gemma3"
//   - "gemma3:latest" -> "gemma3"
//   - "hf.co/bartowski/model:latest" -> "hf.co/bartowski/model"
func stripDefaultsFromModelName(model string) string {
	// Check if model has ai/ prefix without tag (implicitly :latest) - strip just ai/
	if strings.HasPrefix(model, defaultOrg+"/") {
		model = strings.TrimPrefix(model, defaultOrg+"/")
	}

	// Check if model has :latest but no slash (no org specified) - strip :latest
	if strings.HasSuffix(model, ":"+defaultTag) {
		model = strings.TrimSuffix(model, ":"+defaultTag)
	}

	// For other cases (ai/ with custom tag, custom org with :latest, etc.), keep as-is
	return model
}
