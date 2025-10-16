package commands

import (
	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/spf13/cobra"
)

func newStartRunner() *cobra.Command {
	var port uint16
	var gpuMode string
	var doNotTrack bool
	var ollama bool
	c := &cobra.Command{
		Use:   "start-runner",
		Short: "Start Docker Model Runner (Docker Engine only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInstallOrStart(cmd, runnerOptions{
				port:       port,
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
	c.Flags().StringVar(&gpuMode, "gpu", "auto", "Specify GPU support (none|auto|cuda|rocm)")
	c.Flags().BoolVar(&doNotTrack, "do-not-track", false, "Do not track models usage in Docker Model Runner")
	c.Flags().BoolVar(&ollama, "ollama", false, "Use ollama runner instead of Docker Model Runner")
	return c
}
