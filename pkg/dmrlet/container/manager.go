// Package container provides container lifecycle management for dmrlet.
package container

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	containerd_runtime "github.com/docker/model-runner/pkg/dmrlet/runtime"
)

// ContainerState represents the state of a container.
type ContainerState string

const (
	StateCreating ContainerState = "creating"
	StateRunning  ContainerState = "running"
	StateStopped  ContainerState = "stopped"
	StateFailed   ContainerState = "failed"
	StateUnknown  ContainerState = "unknown"
)

// ContainerInfo holds information about a running container.
type ContainerInfo struct {
	ID        string
	Model     string
	Backend   string
	Image     string
	Port      int
	GPUs      []int
	State     ContainerState
	CreatedAt time.Time
	StartedAt time.Time
	Endpoint  string
	Pid       uint32
	ExitCode  int
	Labels    map[string]string
}

// ContainerOpts configures a new container.
type ContainerOpts struct {
	ID          string
	Model       string
	Backend     string
	Image       string
	ModelPath   string
	Port        int
	GPUs        []int
	GPUEnvVars  map[string]string
	ExtraEnv    map[string]string
	Command     []string
	Args        []string
	MemoryLimit int64
	CPULimit    float64
	Labels      map[string]string
}

// Runtime defines the interface for container runtime backends.
type Runtime interface {
	Create(ctx context.Context, opts ContainerOpts) (string, error)
	Start(ctx context.Context, id string) error
	Stop(ctx context.Context, id string, timeout time.Duration) error
	Remove(ctx context.Context, id string) error
	Logs(ctx context.Context, id string, follow bool) (io.ReadCloser, error)
	Inspect(ctx context.Context, id string) (*ContainerInfo, error)
	List(ctx context.Context) ([]ContainerInfo, error)
	PullImage(ctx context.Context, image string) error
}

// Manager manages container lifecycle.
type Manager struct {
	mu         sync.RWMutex
	runtime    Runtime
	containers map[string]*ContainerInfo
}

// NewManager creates a new container manager.
func NewManager(address string) (*Manager, error) {
	// Check environment variable to determine runtime to use
	runtimeType := os.Getenv("DMRLET_RUNTIME")
	if runtimeType == "" {
		runtimeType = "containerd" // default
	}

	var runtime Runtime
	var err error

	switch runtimeType {
	case "docker":
		// Use Docker CLI runtime
		runtime = &DockerRuntime{}
	case "containerd", "":
		// Use containerd runtime
		runtime, err = NewContainerDRuntime(address)
		if err != nil {
			return nil, fmt.Errorf("failed to create containerd runtime: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported runtime type: %s", runtimeType)
	}

	return &Manager{
		runtime:    runtime,
		containers: make(map[string]*ContainerInfo),
	}, nil
}

// Connect establishes connection to the container runtime.
func (m *Manager) Connect(ctx context.Context) error {
	// Docker runtime doesn't need explicit connection
	return nil
}

// Close closes the runtime connection.
func (m *Manager) Close() error {
	return nil
}

// PullImage pulls an image from a registry.
func (m *Manager) PullImage(ctx context.Context, ref string) error {
	return m.runtime.PullImage(ctx, ref)
}

// Create creates and starts a new container.
func (m *Manager) Create(ctx context.Context, opts ContainerOpts) (*ContainerInfo, error) {
	// Create container
	id, err := m.runtime.Create(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	// Start container
	if err := m.runtime.Start(ctx, id); err != nil {
		m.runtime.Remove(ctx, id)
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	info := &ContainerInfo{
		ID:        id,
		Model:     opts.Model,
		Backend:   opts.Backend,
		Image:     opts.Image,
		Port:      opts.Port,
		GPUs:      extractGPUIndices(opts.GPUEnvVars),
		State:     StateRunning,
		CreatedAt: time.Now(),
		StartedAt: time.Now(),
		Endpoint:  fmt.Sprintf("localhost:%d", opts.Port),
		Labels:    opts.Labels,
	}

	m.mu.Lock()
	m.containers[id] = info
	m.mu.Unlock()

	return info, nil
}

// Stop stops a running container.
func (m *Manager) Stop(ctx context.Context, id string, timeout time.Duration) error {
	if err := m.runtime.Stop(ctx, id, timeout); err != nil {
		return err
	}

	m.mu.Lock()
	if info, ok := m.containers[id]; ok {
		info.State = StateStopped
	}
	m.mu.Unlock()

	return nil
}

// Remove removes a container.
func (m *Manager) Remove(ctx context.Context, id string) error {
	if err := m.runtime.Remove(ctx, id); err != nil {
		return err
	}

	m.mu.Lock()
	delete(m.containers, id)
	m.mu.Unlock()

	return nil
}

// Restart restarts a container.
func (m *Manager) Restart(ctx context.Context, id string) error {
	m.mu.RLock()
	info, ok := m.containers[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("container %s not found", id)
	}

	// Stop the container
	if err := m.Stop(ctx, id, 10*time.Second); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	// Remove and recreate
	if err := m.Remove(ctx, id); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}

	// Recreate with same options
	opts := ContainerOpts{
		ID:      id,
		Model:   info.Model,
		Backend: info.Backend,
		Image:   info.Image,
		Port:    info.Port,
		Labels:  info.Labels,
	}

	_, err := m.Create(ctx, opts)
	return err
}

// List returns all managed containers.
func (m *Manager) List() []ContainerInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []ContainerInfo
	for _, info := range m.containers {
		result = append(result, *info)
	}
	return result
}

// Get returns information about a specific container.
func (m *Manager) Get(id string) (*ContainerInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info, ok := m.containers[id]
	if !ok {
		return nil, false
	}
	infoCopy := *info
	return &infoCopy, true
}

// GetByModel returns all containers running a specific model.
func (m *Manager) GetByModel(model string) []ContainerInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []ContainerInfo
	for _, info := range m.containers {
		if info.Model == model {
			result = append(result, *info)
		}
	}
	return result
}

// AttachLogs attaches to container logs.
func (m *Manager) AttachLogs(ctx context.Context, id string, stdout, stderr io.Writer) error {
	logs, err := m.runtime.Logs(ctx, id, true)
	if err != nil {
		return err
	}
	defer logs.Close()

	_, err = io.Copy(stdout, logs)
	return err
}

// UpdateState updates the state of a container.
func (m *Manager) UpdateState(id string, state ContainerState) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if info, ok := m.containers[id]; ok {
		info.State = state
	}
}

// DockerRuntime implements Runtime using Docker CLI.
type DockerRuntime struct{}

// Create creates a container using Docker.
func (r *DockerRuntime) Create(ctx context.Context, opts ContainerOpts) (string, error) {
	args := []string{"create", "--name", opts.ID}

	// Add port mapping
	if opts.Port > 0 {
		args = append(args, "-p", fmt.Sprintf("%d:%d", opts.Port, opts.Port))
	}

	// Add environment variables
	for k, v := range opts.GPUEnvVars {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	for k, v := range opts.ExtraEnv {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	// Add model mount
	if opts.ModelPath != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/models:ro", opts.ModelPath))
	}

	// Add labels
	for k, v := range opts.Labels {
		args = append(args, "--label", fmt.Sprintf("%s=%s", k, v))
	}

	// Add GPU support for NVIDIA
	if _, ok := opts.GPUEnvVars["NVIDIA_VISIBLE_DEVICES"]; ok {
		args = append(args, "--gpus", "all")
	}

	// Add image
	args = append(args, opts.Image)

	// Add command and args
	args = append(args, opts.Command...)
	args = append(args, opts.Args...)

	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker create failed: %s: %w", string(output), err)
	}

	return opts.ID, nil
}

// Start starts a container.
func (r *DockerRuntime) Start(ctx context.Context, id string) error {
	cmd := exec.CommandContext(ctx, "docker", "start", id)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker start failed: %s: %w", string(output), err)
	}
	return nil
}

// Stop stops a container.
func (r *DockerRuntime) Stop(ctx context.Context, id string, timeout time.Duration) error {
	cmd := exec.CommandContext(ctx, "docker", "stop", "-t", strconv.Itoa(int(timeout.Seconds())), id)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker stop failed: %s: %w", string(output), err)
	}
	return nil
}

// Remove removes a container.
func (r *DockerRuntime) Remove(ctx context.Context, id string) error {
	cmd := exec.CommandContext(ctx, "docker", "rm", "-f", id)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker rm failed: %s: %w", string(output), err)
	}
	return nil
}

// Logs returns container logs.
func (r *DockerRuntime) Logs(ctx context.Context, id string, follow bool) (io.ReadCloser, error) {
	args := []string{"logs"}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, id)

	cmd := exec.CommandContext(ctx, "docker", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return stdout, nil
}

// Inspect returns container information.
func (r *DockerRuntime) Inspect(ctx context.Context, id string) (*ContainerInfo, error) {
	cmd := exec.CommandContext(ctx, "docker", "inspect", "--format",
		"{{.State.Status}}|{{.State.Pid}}|{{.State.ExitCode}}", id)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	parts := strings.Split(strings.TrimSpace(string(output)), "|")
	if len(parts) < 3 {
		return nil, fmt.Errorf("unexpected inspect output")
	}

	state := StateUnknown
	switch parts[0] {
	case "running":
		state = StateRunning
	case "exited":
		state = StateStopped
	case "created":
		state = StateCreating
	}

	pid, _ := strconv.ParseUint(parts[1], 10, 32)
	exitCode, _ := strconv.Atoi(parts[2])

	return &ContainerInfo{
		ID:       id,
		State:    state,
		Pid:      uint32(pid),
		ExitCode: exitCode,
	}, nil
}

// List returns all containers.
func (r *DockerRuntime) List(ctx context.Context) ([]ContainerInfo, error) {
	cmd := exec.CommandContext(ctx, "docker", "ps", "-a", "--filter", "label=dmrlet.model",
		"--format", "{{.ID}}|{{.Names}}|{{.Status}}")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var containers []ContainerInfo
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 3 {
			continue
		}

		state := StateUnknown
		if strings.HasPrefix(parts[2], "Up") {
			state = StateRunning
		} else if strings.HasPrefix(parts[2], "Exited") {
			state = StateStopped
		}

		containers = append(containers, ContainerInfo{
			ID:    parts[0],
			State: state,
		})
	}

	return containers, nil
}

// PullImage pulls an image.
func (r *DockerRuntime) PullImage(ctx context.Context, image string) error {
	cmd := exec.CommandContext(ctx, "docker", "pull", image)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker pull failed: %s: %w", string(output), err)
	}
	return nil
}

// extractGPUIndices extracts GPU indices from environment variables.
func extractGPUIndices(envVars map[string]string) []int {
	var indices []int

	// Check NVIDIA
	if devices, ok := envVars["NVIDIA_VISIBLE_DEVICES"]; ok && devices != "" {
		indices = parseIndices(devices)
	}

	// Check AMD
	if devices, ok := envVars["HIP_VISIBLE_DEVICES"]; ok && devices != "" {
		indices = parseIndices(devices)
	}

	return indices
}

// ContainerDRuntime wraps the containerd runtime implementation
type ContainerDRuntime struct {
	client *containerd_runtime.Runtime
}

// NewContainerDRuntime creates a new containerd runtime
func NewContainerDRuntime(address string) (*ContainerDRuntime, error) {
	ctx := context.Background()

	// Use provided address or default
	socket := address
	if socket == "" {
		socket = containerd_runtime.DefaultContainerdSocket
	}

	runtime, err := containerd_runtime.NewRuntime(ctx, containerd_runtime.WithSocket(socket))
	if err != nil {
		return nil, err
	}

	return &ContainerDRuntime{
		client: runtime,
	}, nil
}

// Create creates a container using containerd.
func (r *ContainerDRuntime) Create(ctx context.Context, opts ContainerOpts) (string, error) {
	// Convert ContainerOpts to containerd ContainerSpec
	spec := containerd_runtime.ContainerSpec{
		ID:      opts.ID,
		Image:   opts.Image,
		Command: append(opts.Command, opts.Args...), // Combine command and args
		Env:     []string{},
		Mounts:  []containerd_runtime.Mount{},
		HostNet: false,
	}

	// Add environment variables
	for k, v := range opts.GPUEnvVars {
		spec.Env = append(spec.Env, fmt.Sprintf("%s=%s", k, v))
	}
	for k, v := range opts.ExtraEnv {
		spec.Env = append(spec.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Add model mount
	if opts.ModelPath != "" {
		spec.Mounts = append(spec.Mounts, containerd_runtime.Mount{
			Source:      opts.ModelPath,
			Destination: "/models",
			ReadOnly:    true,
		})
	}

	// Add GPU devices if specified
	if len(opts.GPUs) > 0 {
		// For now, we'll use environment variables to pass GPU info
		// In a real implementation, we'd add the actual GPU devices
		gpuInfo := &containerd_runtime.GPUInfo{
			Type:    "nvidia",   // Default to nvidia
			Devices: []string{}, // Would populate with actual device paths
		}
		spec.GPU = gpuInfo
	}

	// Run the container
	if err := r.client.Run(ctx, spec); err != nil {
		return "", fmt.Errorf("failed to create container with containerd: %w", err)
	}

	return opts.ID, nil
}

// Start starts a container.
func (r *ContainerDRuntime) Start(ctx context.Context, id string) error {
	// Containerd starts the container during creation, so this is a no-op
	return nil
}

// Stop stops a container.
func (r *ContainerDRuntime) Stop(ctx context.Context, id string, timeout time.Duration) error {
	return r.client.Stop(ctx, id)
}

// Remove removes a container.
func (r *ContainerDRuntime) Remove(ctx context.Context, id string) error {
	return r.client.Stop(ctx, id)
}

// Logs returns container logs.
func (r *ContainerDRuntime) Logs(ctx context.Context, id string, follow bool) (io.ReadCloser, error) {
	// The containerd runtime doesn't have a direct logs method
	// This would need to be implemented in the runtime package
	return nil, fmt.Errorf("logs not implemented for containerd runtime")
}

// Inspect returns container information.
func (r *ContainerDRuntime) Inspect(ctx context.Context, id string) (*ContainerInfo, error) {
	// The containerd runtime doesn't have a direct inspect method
	// This would need to be implemented in the runtime package
	return nil, fmt.Errorf("inspect not implemented for containerd runtime")
}

// List returns all containers.
func (r *ContainerDRuntime) List(ctx context.Context) ([]ContainerInfo, error) {
	containers, err := r.client.List(ctx)
	if err != nil {
		return nil, err
	}

	var result []ContainerInfo
	for _, c := range containers {
		result = append(result, ContainerInfo{
			ID:    c.ID,
			State: StateRunning, // Simplified for now
		})
	}

	return result, nil
}

// PullImage pulls an image.
func (r *ContainerDRuntime) PullImage(ctx context.Context, image string) error {
	return r.client.PullImage(ctx, image)
}

func parseIndices(s string) []int {
	var indices []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var idx int
		if _, err := fmt.Sscanf(part, "%d", &idx); err == nil {
			indices = append(indices, idx)
		}
	}
	return indices
}
