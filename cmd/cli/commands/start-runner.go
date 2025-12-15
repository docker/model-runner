package commands

import (
	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/spf13/cobra"
)

func newStartRunner() *cobra.Command {
	var port uint16
	var gpuMode string
	var backend string
	var doNotTrack bool
	var debug bool
	var proxyCert string
	c := &cobra.Command{
		Use:   "start-runner",
		Short: "Start Docker Model Runner (Docker Engine only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInstallOrStart(cmd, runnerOptions{
				port:       port,
				gpuMode:    gpuMode,
				backend:    backend,
				doNotTrack: doNotTrack,
				pullImage:  false,
				proxyCert:  proxyCert,
			}, debug)
		},
		ValidArgsFunction: completion.NoComplete,
	}
	addRunnerFlags(c, runnerFlagOptions{
		Port:       &port,
		GpuMode:    &gpuMode,
		Backend:    &backend,
		DoNotTrack: &doNotTrack,
		Debug:      &debug,
		ProxyCert:  &proxyCert,
	})
	return c
}
