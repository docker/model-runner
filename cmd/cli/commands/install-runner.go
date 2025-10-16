package commands

import (
	"context"
	"errors"
	"fmt"
	"github.com/docker/model-runner/cmd/cli/pkg/types"
	"os"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/cmd/cli/desktop"
	gpupkg "github.com/docker/model-runner/cmd/cli/pkg/gpu"
	"github.com/docker/model-runner/cmd/cli/pkg/standalone"
	"github.com/spf13/cobra"
)

const (
	// installWaitTries controls how many times the automatic installation will
	// try to reach the model runner while waiting for it to be ready.
	installWaitTries = 20
	// installWaitRetryInterval controls the interval at which automatic
	// installation will try to reach the model runner while waiting for it to
	// be ready.
	installWaitRetryInterval = 500 * time.Millisecond
)

// waitForStandaloneRunnerAfterInstall waits for a standalone model runner
// container to come online after installation. The CPU version can take about a
// second to start serving requests once the container has started, the CUDA
// version can take several seconds.
func waitForStandaloneRunnerAfterInstall(ctx context.Context) error {
	for tries := installWaitTries; tries > 0; tries-- {
		if status := desktopClient.Status(); status.Error == nil && status.Running {
			return nil
		}
		select {
		case <-time.After(installWaitRetryInterval):
		case <-ctx.Done():
			return errors.New("cancelled waiting for standalone model runner to initialize")
		}
	}
	return errors.New("standalone model runner took too long to initialize")
}

// standaloneRunner encodes the standalone runner configuration, if one exists.
type standaloneRunner struct {
	// hostPort is the port that the runner is listening to on the host.
	hostPort uint16
	// gatewayIP is the gateway IP address that the runner is listening on.
	gatewayIP string
	// gatewayPort is the gateway port that the runner is listening on.
	gatewayPort uint16
}

// inspectStandaloneRunner inspects a standalone runner container and extracts
// its configuration.
func inspectStandaloneRunner(container container.Summary) *standaloneRunner {
	result := &standaloneRunner{}
	for _, port := range container.Ports {
		if port.IP == "127.0.0.1" {
			result.hostPort = port.PublicPort
		} else {
			// We don't really have a good way of knowing what the gateway IP
			// address is, but in the standard standalone configuration we only
			// bind to two interfaces: 127.0.0.1 and the gateway interface.
			result.gatewayIP = port.IP
			result.gatewayPort = port.PublicPort
		}
	}
	return result
}

// ensureOllamaRunnerAvailable is a utility function that ensures an ollama runner
// is available. Unlike the regular runner, ollama runners are always used via
// controller containers on all platforms.
func ensureOllamaRunnerAvailable(ctx context.Context, printer standalone.StatusPrinter) (*standaloneRunner, error) {
	// For ollama, we always use controller containers on all platforms
	// If automatic installation has been disabled, then don't do anything.
	if os.Getenv("MODEL_RUNNER_NO_AUTO_INSTALL") != "" {
		return nil, nil
	}

	// Ensure that the output printer is non-nil.
	if printer == nil {
		printer = standalone.NoopPrinter()
	}

	// Create a Docker client for the active context.
	dockerClient, err := desktop.DockerClientForContext(dockerCLI, dockerCLI.CurrentContext())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	// Check if an ollama runner container exists.
	containerID, _, container, err := standalone.FindOllamaControllerContainer(ctx, dockerClient)
	if err != nil {
		return nil, fmt.Errorf("unable to identify existing ollama runner: %w", err)
	} else if containerID != "" {
		return inspectStandaloneRunner(container), nil
	}

	// Automatically determine GPU support.
	gpu, err := gpupkg.ProbeGPUSupport(ctx, dockerClient)
	if err != nil {
		return nil, fmt.Errorf("unable to probe GPU support: %w", err)
	}

	// Ensure that we have an up-to-date copy of the ollama image.
	if err := standalone.EnsureOllamaImage(ctx, dockerClient, gpu, "", printer); err != nil {
		return nil, fmt.Errorf("unable to pull latest ollama image: %w", err)
	}

	// Ensure that we have an ollama storage volume.
	modelStorageVolume, err := standalone.EnsureOllamaStorageVolume(ctx, dockerClient, printer)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize ollama storage: %w", err)
	}

	// Create the ollama runner container.
	port := uint16(standalone.DefaultOllamaPort)
	host := "127.0.0.1"
	engineKind := modelRunner.EngineKind()
	environment := "moby"
	if engineKind == types.ModelRunnerEngineKindCloud {
		environment = "cloud"
	}
	if err := standalone.CreateOllamaControllerContainer(ctx, dockerClient, port, host, environment, false, gpu, "", modelStorageVolume, printer, engineKind); err != nil {
		return nil, fmt.Errorf("unable to initialize ollama container: %w", err)
	}

	// Poll until we get a response from the ollama runner.
	// Note: We reuse the same wait logic, assuming ollama responds similarly
	if err := waitForStandaloneRunnerAfterInstall(ctx); err != nil {
		return nil, err
	}

	// Find the runner container.
	containerID, _, container, err = standalone.FindOllamaControllerContainer(ctx, dockerClient)
	if err != nil {
		return nil, fmt.Errorf("unable to identify existing ollama runner: %w", err)
	} else if containerID == "" {
		return nil, errors.New("ollama runner not found after installation")
	}
	return inspectStandaloneRunner(container), nil
}

// ensureStandaloneRunnerAvailable is a utility function that other commands can
// use to initialize a default standalone model runner. It is a no-op in
// unsupported contexts or if automatic installs have been disabled.
func ensureStandaloneRunnerAvailable(ctx context.Context, printer standalone.StatusPrinter) (*standaloneRunner, error) {
	// If we're not in a supported model runner context, then don't do anything.
	engineKind := modelRunner.EngineKind()
	standaloneSupported := engineKind == types.ModelRunnerEngineKindMoby ||
		engineKind == types.ModelRunnerEngineKindCloud
	if !standaloneSupported {
		return nil, nil
	}

	// If automatic installation has been disabled, then don't do anything.
	if os.Getenv("MODEL_RUNNER_NO_AUTO_INSTALL") != "" {
		return nil, nil
	}

	// Ensure that the output printer is non-nil.
	if printer == nil {
		printer = standalone.NoopPrinter()
	}

	// Create a Docker client for the active context.
	dockerClient, err := desktop.DockerClientForContext(dockerCLI, dockerCLI.CurrentContext())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	// Check if a model runner container exists.
	containerID, _, container, err := standalone.FindControllerContainer(ctx, dockerClient)
	if err != nil {
		return nil, fmt.Errorf("unable to identify existing standalone model runner: %w", err)
	} else if containerID != "" {
		return inspectStandaloneRunner(container), nil
	}

	// Automatically determine GPU support.
	gpu, err := gpupkg.ProbeGPUSupport(ctx, dockerClient)
	if err != nil {
		return nil, fmt.Errorf("unable to probe GPU support: %w", err)
	}

	// Ensure that we have an up-to-date copy of the image.
	if err := standalone.EnsureControllerImage(ctx, dockerClient, gpu, printer); err != nil {
		return nil, fmt.Errorf("unable to pull latest standalone model runner image: %w", err)
	}

	// Ensure that we have a model storage volume.
	modelStorageVolume, err := standalone.EnsureModelStorageVolume(ctx, dockerClient, printer)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize standalone model storage: %w", err)
	}

	// Create the model runner container.
	port := uint16(standalone.DefaultControllerPortMoby)
	// For auto-installation, always bind to localhost for security.
	// Users can run install-runner explicitly with --host to change this.
	host := "127.0.0.1"
	environment := "moby"
	if engineKind == types.ModelRunnerEngineKindCloud {
		port = standalone.DefaultControllerPortCloud
		environment = "cloud"
	}
	if err := standalone.CreateControllerContainer(ctx, dockerClient, port, host, environment, false, gpu, modelStorageVolume, printer, engineKind); err != nil {
		return nil, fmt.Errorf("unable to initialize standalone model runner container: %w", err)
	}

	// Poll until we get a response from the model runner.
	if err := waitForStandaloneRunnerAfterInstall(ctx); err != nil {
		return nil, err
	}

	// Find the runner container.
	//
	// TODO: We should actually find this before calling
	// waitForStandaloneRunnerAfterInstall (or have CreateControllerContainer
	// return the container information), and probably pass the target
	// information info waitForStandaloneRunnerAfterInstall, but let's wait
	// until we do listener port customization / detection in the next PR.
	containerID, _, container, err = standalone.FindControllerContainer(ctx, dockerClient)
	if err != nil {
		return nil, fmt.Errorf("unable to identify existing standalone model runner: %w", err)
	} else if containerID == "" {
		return nil, errors.New("standalone model runner not found after installation")
	}
	return inspectStandaloneRunner(container), nil
}

// runnerOptions holds common configuration for install/start/reinstall commands
type runnerOptions struct {
	port            uint16
	host            string
	gpuMode         string
	doNotTrack      bool
	pullImage       bool
	pruneContainers bool
	ollama          bool
}

// runInstallOrStart is shared logic for install-runner and start-runner commands
func runInstallOrStart(cmd *cobra.Command, opts runnerOptions) error {
	// Ensure that we're running in a supported model runner context.
	engineKind := modelRunner.EngineKind()
	if engineKind == types.ModelRunnerEngineKindDesktop {
		// TODO: We may eventually want to auto-forward this to
		// docker desktop enable model-runner, but we should first make
		// sure the CLI flags match.
		cmd.Println("Standalone installation not supported with Docker Desktop")
		cmd.Println("Use `docker desktop enable model-runner` instead")
		return nil
	} else if engineKind == types.ModelRunnerEngineKindMobyManual {
		cmd.Println("Standalone installation not supported with MODEL_RUNNER_HOST set")
		return nil
	}

	port := opts.port
	if port == 0 {
		// Use "0" as a sentinel default flag value so it's not displayed automatically.
		// The default values are written in the usage string.
		// Hence, the user currently won't be able to set the port to 0 in order to get a random available port.
		if opts.ollama {
			port = standalone.DefaultOllamaPort
		} else {
			port = standalone.DefaultControllerPortMoby
		}
	}
	// HACK: If we're in a Cloud context, then we need to use a
	// different default port because it conflicts with Docker Desktop's
	// default model runner host-side port. Unfortunately we can't make
	// the port flag default dynamic (at least not easily) because of
	// when context detection happens. So assume that a default value
	// indicates that we want the Cloud default port. This is less
	// problematic in Cloud since the UX there is mostly invisible.
	if !opts.ollama && engineKind == types.ModelRunnerEngineKindCloud &&
		port == standalone.DefaultControllerPortMoby {
		port = standalone.DefaultControllerPortCloud
	}

	// Set the appropriate environment.
	environment := "moby"
	if engineKind == types.ModelRunnerEngineKindCloud {
		environment = "cloud"
	}

	// Create a Docker client for the active context.
	dockerClient, err := desktop.DockerClientForContext(dockerCLI, dockerCLI.CurrentContext())
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}

	// If pruning containers (reinstall), remove any existing model runner containers.
	if opts.pruneContainers {
		if opts.ollama {
			if err := standalone.PruneOllamaControllerContainers(cmd.Context(), dockerClient, false, cmd); err != nil {
				return fmt.Errorf("unable to remove ollama runner container(s): %w", err)
			}
		} else {
			if err := standalone.PruneControllerContainers(cmd.Context(), dockerClient, false, cmd); err != nil {
				return fmt.Errorf("unable to remove model runner container(s): %w", err)
			}
		}
	} else {
		// Check if an active model runner container already exists (install only).
		var ctrID, ctrName string
		var err error
		if opts.ollama {
			ctrID, ctrName, _, err = standalone.FindOllamaControllerContainer(cmd.Context(), dockerClient)
		} else {
			ctrID, ctrName, _, err = standalone.FindControllerContainer(cmd.Context(), dockerClient)
		}
		if err != nil {
			return err
		} else if ctrID != "" {
			runnerType := "Model Runner"
			if opts.ollama {
				runnerType = "ollama runner"
			}
			if ctrName != "" {
				cmd.Printf("%s container %s (%s) is already running\n", runnerType, ctrName, ctrID[:12])
			} else {
				cmd.Printf("%s container %s is already running\n", runnerType, ctrID[:12])
			}
			return nil
		}
	}

	// Determine GPU support.
	var gpu gpupkg.GPUSupport
	var gpuVariant string
	if opts.gpuMode == "auto" {
		gpu, err = gpupkg.ProbeGPUSupport(cmd.Context(), dockerClient)
		if err != nil {
			return fmt.Errorf("unable to probe GPU support: %w", err)
		}
	} else if opts.gpuMode == "cuda" {
		gpu = gpupkg.GPUSupportCUDA
	} else if opts.gpuMode == "rocm" {
		gpu = gpupkg.GPUSupportROCm
		gpuVariant = "rocm"
	} else if opts.gpuMode != "none" {
		return fmt.Errorf("unknown GPU specification: %q", opts.gpuMode)
	}

	// Ensure that we have an up-to-date copy of the image, if requested.
	if opts.pullImage {
		if opts.ollama {
			if err := standalone.EnsureOllamaImage(cmd.Context(), dockerClient, gpu, gpuVariant, cmd); err != nil {
				return fmt.Errorf("unable to pull latest ollama image: %w", err)
			}
		} else {
			if err := standalone.EnsureControllerImage(cmd.Context(), dockerClient, gpu, cmd); err != nil {
				return fmt.Errorf("unable to pull latest standalone model runner image: %w", err)
			}
		}
	}

	// Ensure that we have a model storage volume.
	var modelStorageVolume string
	if opts.ollama {
		modelStorageVolume, err = standalone.EnsureOllamaStorageVolume(cmd.Context(), dockerClient, cmd)
		if err != nil {
			return fmt.Errorf("unable to initialize ollama storage: %w", err)
		}
	} else {
		modelStorageVolume, err = standalone.EnsureModelStorageVolume(cmd.Context(), dockerClient, cmd)
		if err != nil {
			return fmt.Errorf("unable to initialize standalone model storage: %w", err)
		}
	}

	// Create the model runner container.
	if opts.ollama {
		if err := standalone.CreateOllamaControllerContainer(cmd.Context(), dockerClient, port, opts.host, environment, opts.doNotTrack, gpu, gpuVariant, modelStorageVolume, cmd, engineKind); err != nil {
			return fmt.Errorf("unable to initialize ollama container: %w", err)
		}
	} else {
		if err := standalone.CreateControllerContainer(cmd.Context(), dockerClient, port, opts.host, environment, opts.doNotTrack, gpu, modelStorageVolume, cmd, engineKind); err != nil {
			return fmt.Errorf("unable to initialize standalone model runner container: %w", err)
		}
	}

	// Poll until we get a response from the model runner.
	return waitForStandaloneRunnerAfterInstall(cmd.Context())
}

func newInstallRunner() *cobra.Command {
	var port uint16
	var host string
	var gpuMode string
	var doNotTrack bool
	var ollama bool
	c := &cobra.Command{
		Use:   "install-runner",
		Short: "Install Docker Model Runner (Docker Engine only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInstallOrStart(cmd, runnerOptions{
				port:            port,
				host:            host,
				gpuMode:         gpuMode,
				doNotTrack:      doNotTrack,
				pullImage:       true,
				pruneContainers: false,
				ollama:          ollama,
			})
		},
		ValidArgsFunction: completion.NoComplete,
	}
	c.Flags().Uint16Var(&port, "port", 0,
		"Docker container port for Docker Model Runner (default: 12434 for Docker Engine, 12435 for Cloud mode, 11434 for ollama)")
	c.Flags().StringVar(&host, "host", "127.0.0.1", "Host address to bind Docker Model Runner")
	c.Flags().StringVar(&gpuMode, "gpu", "auto", "Specify GPU support (none|auto|cuda|rocm)")
	c.Flags().BoolVar(&doNotTrack, "do-not-track", false, "Do not track models usage in Docker Model Runner")
	c.Flags().BoolVar(&ollama, "ollama", false, "Use ollama runner instead of Docker Model Runner")
	return c
}
