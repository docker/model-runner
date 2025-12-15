package commands

import (
	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/spf13/cobra"
)

func newRestartRunner() *cobra.Command {
	var port uint16
	var host string
	var gpuMode string
	var doNotTrack bool
	var debug bool
	var proxyCert string
	c := &cobra.Command{
		Use:   "restart-runner",
		Short: "Restart Docker Model Runner (Docker Engine only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			// First stop the runner without removing models or images
			if err := runUninstallOrStop(cmd, cleanupOptions{
				models:       false,
				removeImages: false,
			}); err != nil {
				return err
			}

			// Then start the runner with the provided options
			return runInstallOrStart(cmd, runnerOptions{
				port:       port,
				host:       host,
				gpuMode:    gpuMode,
				doNotTrack: doNotTrack,
				pullImage:  false,
				proxyCert:  proxyCert,
			}, debug)
		},
		ValidArgsFunction: completion.NoComplete,
	}
	addRunnerFlags(c, runnerFlagOptions{
		Port:       &port,
		Host:       &host,
		GpuMode:    &gpuMode,
		DoNotTrack: &doNotTrack,
		Debug:      &debug,
		ProxyCert:  &proxyCert,
	})
	return c
}
