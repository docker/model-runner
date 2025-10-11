package commands

import (
	"fmt"
	"github.com/docker/model-runner/cmd/cli/pkg/types"

	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/cmd/cli/pkg/standalone"
	"github.com/spf13/cobra"
)

func newStartRunner() *cobra.Command {
	c := &cobra.Command{
		Use:   "start",
		Short: "Start Docker Model Runner container",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Ensure that we're running in a supported model runner context.
			if kind := modelRunner.EngineKind(); kind == types.ModelRunnerEngineKindDesktop {
				cmd.Println("Standalone start not supported with Docker Desktop")
				cmd.Println("Use `docker desktop enable model-runner` instead")
				return nil
			} else if kind == types.ModelRunnerEngineKindMobyManual {
				cmd.Println("Standalone start not supported with MODEL_RUNNER_HOST set")
				return nil
			}

			// Create a Docker client for the active context.
			dockerClient, err := desktop.DockerClientForContext(dockerCLI, dockerCLI.CurrentContext())
			if err != nil {
				return fmt.Errorf("failed to create Docker client: %w", err)
			}

			// Start the model runner container.
			if err := standalone.StartControllerContainer(cmd.Context(), dockerClient, cmd); err != nil {
				return fmt.Errorf("unable to start model runner container: %w", err)
			}

			return nil
		},
		ValidArgsFunction: completion.NoComplete,
	}
	return c
}
