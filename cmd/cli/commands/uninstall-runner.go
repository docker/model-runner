package commands

import (
	"fmt"

	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/cmd/cli/pkg/standalone"
	"github.com/docker/model-runner/cmd/cli/pkg/types"
	"github.com/spf13/cobra"
)

// cleanupOptions holds common configuration for uninstall/stop commands
type cleanupOptions struct {
	models       bool
	removeImages bool
	backend      string
}

// runUninstallOrStop is shared logic for uninstall-runner and stop-runner commands
func runUninstallOrStop(cmd *cobra.Command, opts cleanupOptions) error {
	// Deferred backend uninstall is handled via the running model runner API
	// and works in any context (Desktop, Moby, etc.), so handle it first.
	if opts.backend != "" {
		if err := desktopClient.UninstallBackend(opts.backend); err != nil {
			return fmt.Errorf("failed to uninstall %s backend: %w", opts.backend, err)
		}
		cmd.Printf("Uninstalled %s backend\n", opts.backend)
		return nil
	}

	// Ensure that we're running in a supported model runner context.
	if kind := modelRunner.EngineKind(); kind == types.ModelRunnerEngineKindDesktop {
		if desktop.IsDesktopWSLContext(cmd.Context(), dockerCLI) {
			// In WSL context, treat as Moby for cleanup
		} else {
			// TODO: We may eventually want to auto-forward this to
			// docker desktop disable model-runner, but we should first
			// make install-runner forward in the same way.
			cmd.Println("Standalone uninstallation not supported with Docker Desktop")
			cmd.Println("Use `docker desktop disable model-runner` instead")
			return nil
		}
	}

	// Create a Docker client for the active context.
	dockerClient, err := desktop.DockerClientForContext(dockerCLI, dockerCLI.CurrentContext())
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}

	// Remove any model runner containers.
	if err := standalone.PruneControllerContainers(cmd.Context(), dockerClient, false, asPrinter(cmd)); err != nil {
		return fmt.Errorf("unable to remove model runner container(s): %w", err)
	}

	// Remove model runner images, if requested.
	if opts.removeImages {
		if err := standalone.PruneControllerImages(cmd.Context(), dockerClient, asPrinter(cmd)); err != nil {
			return fmt.Errorf("unable to remove model runner image(s): %w", err)
		}
	}

	// Remove model storage, if requested.
	if opts.models {
		if err := standalone.PruneModelStorageVolumes(cmd.Context(), dockerClient, asPrinter(cmd)); err != nil {
			return fmt.Errorf("unable to remove model storage volume(s): %w", err)
		}
	}

	return nil
}

func newUninstallRunner() *cobra.Command {
	var models, images bool
	var backend string
	c := &cobra.Command{
		Use:   "uninstall-runner",
		Short: "Uninstall Docker Model Runner (Docker Engine only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUninstallOrStop(cmd, cleanupOptions{
				models:       models,
				removeImages: images,
				backend:      backend,
			})
		},
		ValidArgsFunction: completion.NoComplete,
	}
	c.Flags().BoolVar(&models, "models", false, "Remove model storage volume")
	c.Flags().BoolVar(&images, "images", false, "Remove "+standalone.ControllerImage+" images")
	c.Flags().StringVar(&backend, "backend", "", "Uninstall a deferred backend (e.g. vllm, diffusers)")
	return c
}
