package commands

import (
	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/spf13/cobra"
)

func newReinstallRunner() *cobra.Command {
	var port uint16
	var host string
	var gpuMode string
	var backend string
	var doNotTrack bool
	var debug bool
	var proxyCert string
	var tlsEnabled bool
	var tlsPort uint16
	var tlsCert string
	var tlsKey string
	c := &cobra.Command{
		Use:   "reinstall-runner",
		Short: "Reinstall Docker Model Runner (Docker Engine only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInstallOrStart(cmd, runnerOptions{
				port:            port,
				host:            host,
				gpuMode:         gpuMode,
				backend:         backend,
				doNotTrack:      doNotTrack,
				pullImage:       true,
				pruneContainers: true,
				proxyCert:       proxyCert,
				tls:             tlsEnabled,
				tlsPort:         tlsPort,
				tlsCert:         tlsCert,
				tlsKey:          tlsKey,
			}, debug)
		},
		ValidArgsFunction: completion.NoComplete,
	}
	addRunnerFlags(c, runnerFlagOptions{
		Port:       &port,
		Host:       &host,
		GpuMode:    &gpuMode,
		Backend:    &backend,
		DoNotTrack: &doNotTrack,
		Debug:      &debug,
		ProxyCert:  &proxyCert,
		TLS:        &tlsEnabled,
		TLSPort:    &tlsPort,
		TLSCert:    &tlsCert,
		TLSKey:     &tlsKey,
	})
	return c
}
