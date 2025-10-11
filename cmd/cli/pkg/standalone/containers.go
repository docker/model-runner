package standalone

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	gpupkg "github.com/docker/model-runner/cmd/cli/pkg/gpu"
	"github.com/docker/model-runner/cmd/cli/pkg/types"
)

// controllerContainerName is the name to use for the controller container.
const controllerContainerName = "docker-model-runner"

// copyDockerConfigToContainer copies the Docker config file from the host to the container
// and sets up proper ownership and permissions for the modelrunner user.
// It does nothing for Desktop and Cloud engine kinds.
func copyDockerConfigToContainer(ctx context.Context, dockerClient *client.Client, containerID string, engineKind types.ModelRunnerEngineKind) error {
	// Do nothing for Desktop and Cloud engine kinds
	if engineKind == types.ModelRunnerEngineKindDesktop || engineKind == types.ModelRunnerEngineKindCloud ||
		os.Getenv("_MODEL_RUNNER_TREAT_DESKTOP_AS_MOBY") == "1" {
		return nil
	}

	dockerConfigPath := os.ExpandEnv("$HOME/.docker/config.json")
	if s, err := os.Stat(dockerConfigPath); err != nil || s.Mode()&os.ModeType != 0 {
		return nil
	}

	configData, err := os.ReadFile(dockerConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read Docker config file: %w", err)
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	header := &tar.Header{
		Name: ".docker/config.json",
		Mode: 0600,
		Size: int64(len(configData)),
	}
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("failed to write tar header: %w", err)
	}
	if _, err := tw.Write(configData); err != nil {
		return fmt.Errorf("failed to write config data to tar: %w", err)
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("failed to close tar writer: %w", err)
	}

	// Ensure the .docker directory exists
	mkdirCmd := "mkdir -p /home/modelrunner/.docker && chown modelrunner:modelrunner /home/modelrunner/.docker"
	if err := execInContainer(ctx, dockerClient, containerID, mkdirCmd); err != nil {
		return err
	}

	// Copy directly into the .docker directory
	err = dockerClient.CopyToContainer(ctx, containerID, "/home/modelrunner", &buf, container.CopyToContainerOptions{
		CopyUIDGID: true,
	})
	if err != nil {
		return fmt.Errorf("failed to copy config file to container: %w", err)
	}

	// Set correct ownership and permissions
	chmodCmd := "chown modelrunner:modelrunner /home/modelrunner/.docker/config.json && chmod 600 /home/modelrunner/.docker/config.json"
	if err := execInContainer(ctx, dockerClient, containerID, chmodCmd); err != nil {
		return err
	}

	return nil
}

func execInContainer(ctx context.Context, dockerClient *client.Client, containerID, cmd string) error {
	execConfig := container.ExecOptions{
		Cmd: []string{"sh", "-c", cmd},
	}
	execResp, err := dockerClient.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return fmt.Errorf("failed to create exec for command '%s': %w", cmd, err)
	}
	if err := dockerClient.ContainerExecStart(ctx, execResp.ID, container.ExecStartOptions{}); err != nil {
		return fmt.Errorf("failed to start exec for command '%s': %w", cmd, err)
	}

	// Create a timeout context for the polling loop
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Poll until the command finishes or timeout occurs
	for {
		inspectResp, err := dockerClient.ContainerExecInspect(ctx, execResp.ID)
		if err != nil {
			return fmt.Errorf("failed to inspect exec for command '%s': %w", cmd, err)
		}

		if !inspectResp.Running {
			// Command has finished, now we can safely check the exit code
			if inspectResp.ExitCode != 0 {
				return fmt.Errorf("command '%s' failed with exit code %d", cmd, inspectResp.ExitCode)
			}
			return nil
		}

		// Brief sleep to avoid busy polling, with timeout check
		select {
		case <-time.After(100 * time.Millisecond):
			// Continue polling
		case <-timeoutCtx.Done():
			return fmt.Errorf("command '%s' timed out after 10 seconds", cmd)
		}
	}
}

// FindControllerContainer searches for a running controller container. It
// returns the ID of the container (if found), the container name (if any), the
// full container summary (if found), or any error that occurred.
func FindControllerContainer(ctx context.Context, dockerClient client.ContainerAPIClient) (string, string, container.Summary, error) {
	// Before listing, prune any stopped controller containers.
	if err := PruneControllerContainers(ctx, dockerClient, true, NoopPrinter()); err != nil {
		return "", "", container.Summary{}, fmt.Errorf("unable to prune stopped model runner containers: %w", err)
	}

	// Identify all controller containers.
	containers, err := dockerClient.ContainerList(ctx, container.ListOptions{
		Filters: filters.NewArgs(
			// Don't include a value on this first label selector; Docker Cloud
			// middleware only shows these containers if no value is queried.
			filters.Arg("label", labelDesktopService),
			filters.Arg("label", labelRole+"="+roleController),
		),
	})
	if err != nil {
		return "", "", container.Summary{}, fmt.Errorf("unable to identify model runner containers: %w", err)
	}
	if len(containers) == 0 {
		return "", "", container.Summary{}, nil
	}
	var containerName string
	if len(containers[0].Names) > 0 {
		containerName = strings.TrimPrefix(containers[0].Names[0], "/")
	}
	return containers[0].ID, containerName, containers[0], nil
}

// determineBridgeGatewayIP attempts to identify the engine's host gateway IP
// address on the bridge network. It may return an empty IP address even with a
// nil error if no IP could be identified.
func determineBridgeGatewayIP(ctx context.Context, dockerClient client.NetworkAPIClient) (string, error) {
	bridge, err := dockerClient.NetworkInspect(ctx, "bridge", network.InspectOptions{})
	if err != nil {
		return "", err
	}
	for _, config := range bridge.IPAM.Config {
		if config.Gateway != "" {
			return config.Gateway, nil
		}
	}
	return "", nil
}

// ensureContainerStarted ensures that a container has started. It may be called
// concurrently, taking advantage of the fact that ContainerStart is idempotent.
func ensureContainerStarted(ctx context.Context, dockerClient client.ContainerAPIClient, containerID string) error {
	for i := 10; i > 0; i-- {
		err := dockerClient.ContainerStart(ctx, containerID, container.StartOptions{})
		if err == nil {
			return nil
		}
		// There is a small gap between the time that a container ID and
		// name are registered and the time that the container is actually
		// created and shows up in container list and inspect requests:
		//
		// https://github.com/moby/moby/blob/de24c536b0ea208a09e0fff3fd896c453da6ef2e/daemon/container.go#L138-L156
		//
		// Given that multiple install operations tend to end up tightly
		// synchronized by the preceeding pull operation and that this
		// method is specifically designed to work around these race
		// conditions, we'll allow 404 errors to pass silently (at least up
		// until the polling time out - unfortunately we can't make the 404
		// acceptance window any smaller than that because the CUDA-based
		// containers are large and can take time to create).
		//
		// For some reason, this error case can also manifest as an EOF on the
		// request (I'm not sure where this arises in the Moby server), so we'll
		// let that pass silently too.
		// TODO: Investigate whether nvidia runtime actually returns IsNotFound.
		if !(errdefs.IsNotFound(err) || errors.Is(err, io.EOF) || strings.Contains(err.Error(), "No such container")) {
			return err
		}
		if i > 1 {
			select {
			case <-time.After(500 * time.Millisecond):
			case <-ctx.Done():
				return errors.New("waiting cancelled")
			}
		}
	}
	return errors.New("timed out")
}

// CreateControllerContainer creates and starts a controller container.
func CreateControllerContainer(ctx context.Context, dockerClient *client.Client, port uint16, environment string, doNotTrack bool, gpu gpupkg.GPUSupport, modelStorageVolume string, printer StatusPrinter, engineKind types.ModelRunnerEngineKind) error {
	// Determine the target image.
	var imageName string
	switch gpu {
	case gpupkg.GPUSupportCUDA:
		imageName = ControllerImage + ":" + controllerImageTagCUDA()
	default:
		imageName = ControllerImage + ":" + controllerImageTagCPU()
	}

	// Set up the container configuration.
	portStr := strconv.Itoa(int(port))
	env := []string{
		"MODEL_RUNNER_PORT=" + portStr,
		"MODEL_RUNNER_ENVIRONMENT=" + environment,
	}
	if doNotTrack {
		env = append(env, "DO_NOT_TRACK=1")
	}
	config := &container.Config{
		Image: imageName,
		Env:   env,
		ExposedPorts: nat.PortSet{
			nat.Port(portStr + "/tcp"): struct{}{},
		},
		Labels: map[string]string{
			labelDesktopService: serviceModelRunner,
			labelRole:           roleController,
		},
	}
	hostConfig := &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeVolume,
				Source: modelStorageVolume,
				Target: "/models",
			},
		},
		RestartPolicy: container.RestartPolicy{
			Name: "always",
		},
	}
	portBindings := []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: portStr}}
	if os.Getenv("_MODEL_RUNNER_TREAT_DESKTOP_AS_MOBY") != "1" {
		// Don't bind the bridge gateway IP if we're treating Docker Desktop as Moby.
		if bridgeGatewayIP, err := determineBridgeGatewayIP(ctx, dockerClient); err == nil && bridgeGatewayIP != "" {
			portBindings = append(portBindings, nat.PortBinding{HostIP: bridgeGatewayIP, HostPort: portStr})
		}
	}
	hostConfig.PortBindings = nat.PortMap{
		nat.Port(portStr + "/tcp"): portBindings,
	}
	if gpu == gpupkg.GPUSupportCUDA {
		if ok, err := gpupkg.HasNVIDIARuntime(ctx, dockerClient); err == nil && ok {
			hostConfig.Runtime = "nvidia"
		}
		hostConfig.DeviceRequests = []container.DeviceRequest{{Count: -1, Capabilities: [][]string{{"gpu"}}}}
	}

	// devicePaths contains glob patterns for common AI accelerator device files.
	// Enable access to AI accelerator devices if they exist
	devicePaths := []string{
		"/dev/dri",       // Direct Rendering Infrastructure (used by Vulkan, Mesa, Intel/AMD GPUs)
		"/dev/kfd",       // AMD Kernel Fusion Driver (for ROCm)
		"/dev/accel",     // Intel accelerator devices
		"/dev/davinci*",  // TI DaVinci video processors
		"/dev/devmm_svm", // Huawei Ascend NPU
		"/dev/hisi_hdc",  // Huawei Ascend NPU
	}

	for _, path := range devicePaths {
		devices, err := filepath.Glob(path)
		if err != nil {
			// Skip on glob error, don't fail container creation
			continue
		}
		for _, device := range devices {
			hostConfig.Devices = append(hostConfig.Devices, container.DeviceMapping{
				PathOnHost:        device,
				PathInContainer:   device,
				CgroupPermissions: "rwm",
			})
		}
	}

	if runtime.GOOS == "linux" {
		out, err := exec.CommandContext(ctx, "getent", "group", "render").CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to retrieve the GID of 'render': %w", err)
		}
		tokens := strings.Split(string(out), ":")
		gid, err := strconv.Atoi(tokens[2])
		if err != nil {
			return fmt.Errorf("failed to parse the GID of 'render': %w", err)
		}
		hostConfig.GroupAdd = append(hostConfig.GroupAdd, strconv.Itoa(gid))
	}

	// Create the container. If we detect that a concurrent installation is in
	// progress (as indicated by a conflicting container name (which should have
	// been detected just before installation)), then we'll allow the error to
	// pass silently and simply work in conjunction with any concurrent
	// installers to start the container.
	// TODO: Remove strings.Contains check once we ensure it's not necessary.
	resp, err := dockerClient.ContainerCreate(ctx, config, hostConfig, nil, nil, controllerContainerName)
	if err != nil && !(errdefs.IsConflict(err) || strings.Contains(err.Error(), "is already in use by container")) {
		return fmt.Errorf("failed to create container %s: %w", controllerContainerName, err)
	}
	created := err == nil

	// Start the container.
	printer.Printf("Starting model runner container %s...\n", controllerContainerName)
	if err := ensureContainerStarted(ctx, dockerClient, controllerContainerName); err != nil {
		if created {
			_ = dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		}
		return fmt.Errorf("failed to start container %s: %w", controllerContainerName, err)
	}

	// Copy Docker config file if it exists and we're the container creator.
	if created {
		if err := copyDockerConfigToContainer(ctx, dockerClient, resp.ID, engineKind); err != nil {
			// Log warning but continue - don't fail container creation
			printer.Printf("Warning: failed to copy Docker config: %v\n", err)
		}
	}
	return nil
}

// StartControllerContainer starts the model runner controller container.
func StartControllerContainer(ctx context.Context, dockerClient client.ContainerAPIClient, printer StatusPrinter) error {
	// Find the controller container.
	containerID, containerName, _, err := FindControllerContainer(ctx, dockerClient)
	if err != nil {
		return fmt.Errorf("unable to find model runner container: %w", err)
	}
	if containerID == "" {
		return errors.New("model runner container not found")
	}

	// Start the container.
	if containerName != "" {
		printer.Printf("Starting model runner container %s (%s)...\n", containerName, containerID[:12])
	} else {
		printer.Printf("Starting model runner container %s...\n", containerID[:12])
	}
	if err := ensureContainerStarted(ctx, dockerClient, containerID); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	printer.Printf("Model runner container started successfully\n")
	return nil
}

// StopControllerContainer stops the model runner controller container.
func StopControllerContainer(ctx context.Context, dockerClient client.ContainerAPIClient, printer StatusPrinter) error {
	// Find the controller container.
	containerID, containerName, _, err := FindControllerContainer(ctx, dockerClient)
	if err != nil {
		return fmt.Errorf("unable to find model runner container: %w", err)
	}
	if containerID == "" {
		return errors.New("model runner container not found")
	}

	// Stop the container.
	if containerName != "" {
		printer.Printf("Stopping model runner container %s (%s)...\n", containerName, containerID[:12])
	} else {
		printer.Printf("Stopping model runner container %s...\n", containerID[:12])
	}
	stopTimeout := 10 // seconds
	if err := dockerClient.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &stopTimeout}); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	printer.Printf("Model runner container stopped successfully\n")
	return nil
}

// PruneControllerContainers stops and removes any model runner controller
// containers.
func PruneControllerContainers(ctx context.Context, dockerClient client.ContainerAPIClient, skipRunning bool, printer StatusPrinter) error {
	// Identify all controller containers.
	containers, err := dockerClient.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			// Don't include a value on this first label selector; Docker Cloud
			// middleware only shows these containers if no value is queried.
			filters.Arg("label", labelDesktopService),
			filters.Arg("label", labelRole+"="+roleController),
		),
	})
	if err != nil {
		return fmt.Errorf("unable to identify model runner containers: %w", err)
	}

	// Remove all controller containers.
	for _, ctr := range containers {
		if skipRunning && ctr.State == container.StateRunning {
			continue
		}
		if len(ctr.Names) > 0 {
			printer.Printf("Removing container %s (%s)...\n", strings.TrimPrefix(ctr.Names[0], "/"), ctr.ID[:12])
		} else {
			printer.Printf("Removing container %s...\n", ctr.ID[:12])
		}
		err := dockerClient.ContainerRemove(ctx, ctr.ID, container.RemoveOptions{Force: true})
		if err != nil {
			return fmt.Errorf("failed to remove container %s: %w", ctr.Names[0], err)
		}
	}
	return nil
}
