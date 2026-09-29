package main

import (
	"fmt"
	"os"

	"github.com/docker/cli/cli/command"
	"github.com/docker/model-runner/cmd/cli/commands"
	"github.com/spf13/cobra"
)

// defaultHost is the address dmr's client commands talk to by default: the
// daemon started locally via "dmr serve".
const defaultHost = "http://localhost:12434"

// run assembles the dmr root command and executes it. It always forces
// MODEL_RUNNER_HOST to a local address before building the command tree so
// that command execution takes the "manual host" path
// (ModelRunnerEngineKindMobyManual) in cmd/cli/desktop/context.go and never
// probes for a Docker Desktop installation or a Docker Engine connection.
func run() error {
	if os.Getenv("MODEL_RUNNER_HOST") == "" {
		if err := os.Setenv("MODEL_RUNNER_HOST", defaultHost); err != nil {
			return fmt.Errorf("unable to set MODEL_RUNNER_HOST: %w", err)
		}
	}

	// commands.NewRootCmd builds the full model management command tree
	// (run, ls, pull, rm, ps, inspect, logs, ...) shared with the "docker
	// model" CLI plugin. It requires a *command.DockerCli for historical
	// reasons, but NewDockerCli only sets up local state (I/O streams,
	// ~/.docker/config.json) — it never dials a Docker Engine, so this
	// remains fully standalone.
	cli, err := command.NewDockerCli()
	if err != nil {
		return fmt.Errorf("unable to initialize CLI: %w", err)
	}

	root := commands.NewRootCmd(cli)
	root.Use = "dmr"
	root.Short = "Docker Model Runner"
	root.Long = `dmr is the standalone Docker Model Runner.

It runs both the inference daemon ("dmr serve") and the client CLI used to
manage and run models, with no dependency on Docker Desktop or a running
Docker Engine.`
	root.Version = Version

	root.AddCommand(newServeCmd())
	removeEngineOnlyCommands(root)

	return root.Execute()
}

// engineOnlyCommands manage a Docker-Engine-hosted model-runner container
// (pull the image, create/start/stop it, etc.). They don't apply to
// standalone dmr, which runs the daemon in-process via "dmr serve" instead,
// so they're removed from the command tree to avoid confusion.
var engineOnlyCommands = []string{
	"install-runner",
	"uninstall-runner",
	"start-runner",
	"stop-runner",
	"restart-runner",
	"reinstall-runner",
}

func removeEngineOnlyCommands(root *cobra.Command) {
	names := make(map[string]bool, len(engineOnlyCommands))
	for _, name := range engineOnlyCommands {
		names[name] = true
	}

	var toRemove []*cobra.Command
	for _, cmd := range root.Commands() {
		if names[cmd.Name()] {
			toRemove = append(toRemove, cmd)
		}
	}
	root.RemoveCommand(toRemove...)
}
