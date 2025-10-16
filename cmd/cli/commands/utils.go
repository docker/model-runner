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
	defaultOrg = "ai"
	defaultTag = "latest"
)

const (
	enableViaCLI = "Enable Docker Model Runner via the CLI → docker desktop enable model-runner"
	enableViaGUI = "Enable Docker Model Runner via the GUI → Go to Settings->AI->Enable Docker Model Runner"
)

var notRunningErr = fmt.Errorf("Docker Model Runner is not running. Please start it and try again.\n")

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

// overrideModelRunnerContext updates the model runner context with custom host/port
func overrideModelRunnerContext(host string, port int) error {
	if host == "" {
		host = "127.0.0.1"
	}
	if port == 0 {
		port = 12434
	}

	// Create a new model runner context with the custom host and port
	newContext, err := desktop.NewContextWithHostPort(dockerCLI, host, port)
	if err != nil {
		return fmt.Errorf("unable to create model runner context with host %s and port %d: %w", host, port, err)
	}

	// Update global variables
	modelRunner = newContext
	desktopClient = desktop.New(newContext)

	return nil
}

// overrideModelRunnerContextFromURL updates the model runner context with a custom URL
func overrideModelRunnerContextFromURL(url string, external bool) error {
	// Create a new model runner context with the custom URL
	newContext, err := desktop.NewContextWithURLExternal(dockerCLI, url, external)
	if err != nil {
		return fmt.Errorf("unable to create model runner context with URL %s: %w", url, err)
	}

	// Update global variables
	modelRunner = newContext
	desktopClient = desktop.New(newContext)

	return nil
}
