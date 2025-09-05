package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"

	"github.com/kolesnikovae/go-winjob"
)

// limitTokenMatcher finds limit tokens in a sandbox configuration.
var limitTokenMatcher = regexp.MustCompile(`\(With[a-zA-Z]+\)`)

// limitTokenToGenerator maps limit tokens to their corresponding generators.
var limitTokenToGenerator = map[string]func() winjob.Limit{
	"(WithDesktopLimit)":            winjob.WithDesktopLimit,
	"(WithDieOnUnhandledException)": winjob.WithDieOnUnhandledException,
	"(WithDisplaySettingsLimit)":    winjob.WithDisplaySettingsLimit,
	"(WithExitWindowsLimit)":        winjob.WithExitWindowsLimit,
	"(WithGlobalAtomsLimit)":        winjob.WithGlobalAtomsLimit,
	"(WithHandlesLimit)":            winjob.WithHandlesLimit,
	"(WithDisableOutgoingNetworking)": func() winjob.Limit {
		return winjob.WithOutgoingBandwidthLimit(0)
	},
	"(WithReadClipboardLimit)":    winjob.WithReadClipboardLimit,
	"(WithSystemParametersLimit)": winjob.WithSystemParametersLimit,
	"(WithWriteClipboardLimit)":   winjob.WithWriteClipboardLimit,
}

// ConfigurationLlamaCpp is the sandbox configuration for llama.cpp processes.
const ConfigurationLlamaCpp = `(WithDesktopLimit)
(WithDieOnUnhandledException)
(WithDisplaySettingsLimit)
(WithExitWindowsLimit)
(WithGlobalAtomsLimit)
(WithHandlesLimit)
(WithDisableOutgoingNetworking)
(WithReadClipboardLimit)
(WithSystemParametersLimit)
(WithWriteClipboardLimit)
`

// sandbox is the Windows sandbox implementation.
type sandbox struct {
	// job is the Windows Job object that encapsulates the process.
	job *winjob.JobObject
	// command is the sandboxed process handle.
	command *exec.Cmd
}

// Command implements Sandbox.Command.
func (s *sandbox) Command() *exec.Cmd {
	return s.command
}

// Command implements Sandbox.Close.
func (s *sandbox) Close() error {
	return s.job.Close()
}

// Create creates a sandbox containing a single process that has been started.
// The ctx, name, and arg arguments correspond to their counterparts in
// os/exec.CommandContext. The configuration argument specifies the sandbox
// configuration, for which a pre-defined value should be used. The modifier
// function allows for an optional callback (which may be nil) to configure the
// command before it is started.
func Create(ctx context.Context, configuration string, modifier func(*exec.Cmd), updatedBinPath, name string, arg ...string) (Sandbox, error) {
	// Parse the configuration and configure limits.
	limits := []winjob.Limit{winjob.WithKillOnJobClose()}
	tokens := limitTokenMatcher.FindAllString(configuration, -1)
	for _, token := range tokens {
		if generator, ok := limitTokenToGenerator[token]; ok {
			limits = append(limits, generator())
		} else {
			return nil, fmt.Errorf("unknown limit token: %q", token)
		}
	}

	// Create and configure the command.
	command := exec.CommandContext(ctx, name, arg...)
	if modifier != nil {
		modifier(command)
	}

	// Create the and start the job.
	job, err := winjob.Start(command, limits...)
	if err != nil {
		return nil, fmt.Errorf("unable to start sandboxed process: %w", err)
	}
	return &sandbox{
		job:     job,
		command: command,
	}, nil
}
