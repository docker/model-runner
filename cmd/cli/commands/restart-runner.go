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
	var ollama bool
	c := &cobra.Command{
		Use:   "restart-runner",
		Short: "Restart Docker Model Runner (Docker Engine only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			// First stop the runner without removing models or images
			if err := runUninstallOrStop(cmd, cleanupOptions{
				models:       false,
				removeImages: false,
				ollama:       ollama,
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
				ollama:     ollama,
			})
		},
		ValidArgsFunction: completion.NoComplete,
	}
	c.Flags().Uint16Var(&port, "port", 0,
		"Docker container port for Docker Model Runner (default: 12434 for Docker Engine, 12435 for Cloud mode, 11434 for ollama)")
	c.Flags().StringVar(&host, "host", "127.0.0.1", "Host address to bind Docker Model Runner")
	c.Flags().StringVar(&gpuMode, "gpu", "auto", "Specify GPU support (none|auto|cuda|rocm)")
	c.Flags().BoolVar(&doNotTrack, "do-not-track", false, "Do not track models usage in Docker Model Runner")
	c.Flags().BoolVar(&ollama, "ollama", false, "Restart ollama runner instead of Docker Model Runner")
	return c
}
