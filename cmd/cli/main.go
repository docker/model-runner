package main

import (
	"fmt"
	"os"

	"github.com/docker/cli/cli-plugins/manager"
	"github.com/docker/cli/cli-plugins/plugin"
	"github.com/docker/cli/cli/command"
	"github.com/docker/model-runner/cmd/cli/commands"
	"github.com/docker/model-runner/cmd/cli/desktop"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cli, err := command.NewDockerCli()
	if err != nil {
		return fmt.Errorf("unable to initialize CLI: %w", err)
	}

	rootCmd := commands.NewRootCmd(cli)

	if plugin.RunningStandalone() {
		return rootCmd.Execute()
	}

	return plugin.RunPlugin(cli, rootCmd, manager.Metadata{
		SchemaVersion: "0.1.0",
		Vendor:        "Docker Inc.",
		Version:       desktop.Version,
	})
}
