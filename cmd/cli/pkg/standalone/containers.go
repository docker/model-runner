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
	if err := execInContainer(ctx, dockerClient, containerID, mkdirCmd, false); err != nil {
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
	if err := execInContainer(ctx, dockerClient, containerID, chmodCmd, false); err != nil {
		return err
	}

	return nil
}

func execInContainer(ctx context.Context, dockerClient *client.Client, containerID, cmd string, asRoot bool) error {
	execConfig := container.ExecOptions{
		Cmd: []string{"sh", "-c", cmd},
	}
	if asRoot {
		execConfig.User = "root"
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
		// synchronized by the preceding pull operation and that this
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
		if !errdefs.IsNotFound(err) && !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "No such container") {
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

// isRootless detects if Docker is running in rootless mode.
func isRootless(ctx context.Context, dockerClient *client.Client) bool {
	info, err := dockerClient.Info(ctx)
	if err != nil {
		// If we can't get Docker info, assume it's not rootless to preserve old behavior.
		return false
	}
	for _, opt := range info.SecurityOptions {
		if strings.Contains(opt, "rootless") {
			return true
		}
	}
	return false
}

// Check whether the host Ascend driver path exists. If so, create the corresponding mount configuration.
func tryGetBindAscendMounts(printer StatusPrinter, debug bool) []mount.Mount {
	hostPaths := []string{
		"/usr/local/dcmi",
		"/usr/local/bin/npu-smi",
		"/usr/local/Ascend/driver/lib64",
		"/usr/local/Ascend/driver/version.info",
	}

	var newMounts []mount.Mount
	for _, hostPath := range hostPaths {
		matches, err := filepath.Glob(hostPath)
		if err != nil {
			printer.PrintErrf("Error checking glob pattern for %s: %v\n", hostPath, err)
			continue
		}

		if len(matches) > 0 {
			newMount := mount.Mount{
				Type:     mount.TypeBind,
				Source:   hostPath,
				Target:   hostPath,
				ReadOnly: true,
			}
			newMounts = append(newMounts, newMount)
		} else {
			if debug {
				printer.Printf("[NOT FOUND] Ascend driver path does not exist, skipping: %s\n", hostPath)
			}
		}
	}

	return newMounts
}

// proxyCertContainerPath is the path where the proxy certificate will be mounted in the container.
// This location is used by update-ca-certificates to add the cert to the system trust store.
const proxyCertContainerPath = "/usr/local/share/ca-certificates/proxy-ca.crt"

// CreateControllerContainer creates and starts a controller container.
func CreateControllerContainer(ctx context.Context, dockerClient *client.Client, port uint16, host string, environment string, doNotTrack bool, gpu gpupkg.GPUSupport, backend string, modelStorageVolume string, printer StatusPrinter, engineKind types.ModelRunnerEngineKind, debug bool, vllmOnWSL bool, proxyCert string) error {
	imageName := controllerImageName(gpu, backend)

	// Set up the container configuration.
	portStr := strconv.Itoa(int(port))
	env := []string{
		"MODEL_RUNNER_PORT=" + portStr,
		"MODEL_RUNNER_ENVIRONMENT=" + environment,
	}
	if doNotTrack {
		env = append(env, "DO_NOT_TRACK=1")
	}

	// Pass proxy environment variables to the container if they are set
	proxyEnvVars := []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy"}
	for _, proxyVar := range proxyEnvVars {
		if value, ok := os.LookupEnv(proxyVar); ok {
			env = append(env, proxyVar+"="+value)
		}
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
	ascendMounts := tryGetBindAscendMounts(printer, debug)
	if len(ascendMounts) > 0 {
		hostConfig.Mounts = append(hostConfig.Mounts, ascendMounts...)
	}

	if proxyCert != "" {
		hostConfig.Mounts = append(hostConfig.Mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   proxyCert,
			Target:   proxyCertContainerPath,
			ReadOnly: true,
		})
	}

	portBindings := []nat.PortBinding{{HostIP: host, HostPort: portStr}}
	if os.Getenv("_MODEL_RUNNER_TREAT_DESKTOP_AS_MOBY") != "1" {
		// Don't bind the bridge gateway IP if we're treating Docker Desktop as Moby.
		// Only add bridge gateway IP binding if host is 127.0.0.1 and not in rootless mode
		if host == "127.0.0.1" && !isRootless(ctx, dockerClient) && !vllmOnWSL {
			if bridgeGatewayIP, err := determineBridgeGatewayIP(ctx, dockerClient); err == nil && bridgeGatewayIP != "" {
				portBindings = append(portBindings, nat.PortBinding{HostIP: bridgeGatewayIP, HostPort: portStr})
			}
		}
	}
	hostConfig.PortBindings = nat.PortMap{
		nat.Port(portStr + "/tcp"): portBindings,
	}
	switch gpu {
	case gpupkg.GPUSupportNone:
	case gpupkg.GPUSupportCUDA:
		if ok, err := gpupkg.HasNVIDIARuntime(ctx, dockerClient); err == nil && ok {
			hostConfig.Runtime = "nvidia"
		}
		hostConfig.DeviceRequests = []container.DeviceRequest{{Count: -1, Capabilities: [][]string{{"gpu"}}}}
	case gpupkg.GPUSupportROCm:
		if ok, err := gpupkg.HasROCmRuntime(ctx, dockerClient); err == nil && ok {
			hostConfig.Runtime = "rocm"
		}
		// ROCm devices are handled via device paths (/dev/kfd, /dev/dri) which are already added below
	case gpupkg.GPUSupportMUSA:
		if ok, err := gpupkg.HasMTHREADSRuntime(ctx, dockerClient); err == nil && ok {
			hostConfig.Runtime = "mthreads"
		}
	case gpupkg.GPUSupportCANN:
		if ok, err := gpupkg.HasCANNRuntime(ctx, dockerClient); err == nil && ok {
			hostConfig.Runtime = "cann"
		}
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
			printer.Printf("Warning: render group not found, skipping group addition\n")
		} else {
			trimmedOut := strings.TrimSpace(string(out))
			tokens := strings.Split(trimmedOut, ":")
			if len(tokens) < 3 {
				printer.Printf("Warning: unexpected getent output format: %q\n", trimmedOut)
			} else {
				gid, err := strconv.Atoi(tokens[2])
				if err != nil {
					printer.Printf("Warning: failed to parse render GID from %q: %v\n", tokens[2], err)
				} else {
					hostConfig.GroupAdd = append(hostConfig.GroupAdd, strconv.Itoa(gid))
				}
			}
		}
	}

	if vllmOnWSL && gpu == gpupkg.GPUSupportCUDA {
		hostConfig.Mounts = append(hostConfig.Mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   "/usr/lib/wsl/lib",
			Target:   "/usr/lib/wsl/lib",
			ReadOnly: true,
		})

		env = append(env, "LD_LIBRARY_PATH=/usr/lib/wsl/lib:/usr/local/cuda/lib64:${LD_LIBRARY_PATH}")
		config.Env = env
	}

	// Create the container. If we detect that a concurrent installation is in
	// progress (as indicated by a conflicting container name (which should have
	// been detected just before installation)), then we'll allow the error to
	// pass silently and simply work in conjunction with any concurrent
	// installers to start the container.
	// TODO: Remove strings.Contains check once we ensure it's not necessary.
	resp, err := dockerClient.ContainerCreate(ctx, config, hostConfig, nil, nil, controllerContainerName)
	if err != nil && !errdefs.IsConflict(err) && !strings.Contains(err.Error(), "is already in use by container") {
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
	if created && !vllmOnWSL {
		if err := copyDockerConfigToContainer(ctx, dockerClient, resp.ID, engineKind); err != nil {
			// Log warning but continue - don't fail container creation
			printer.Printf("Warning: failed to copy Docker config: %v\n", err)
		}
	}

	// Add proxy certificate to the system CA bundle (requires root for update-ca-certificates)
	if created && proxyCert != "" {
		printer.Printf("Updating CA certificates...\n")
		if err := execInContainer(ctx, dockerClient, resp.ID, "update-ca-certificates", true); err != nil {
			printer.Printf("Warning: failed to update CA certificates: %v\n", err)
		} else {
			printer.Printf("Restarting container to apply CA certificate...\n")
			if err := dockerClient.ContainerRestart(ctx, resp.ID, container.StopOptions{}); err != nil {
				printer.Printf("Warning: failed to restart container after adding CA certificate: %v\n", err)
			}
		}
	}

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
