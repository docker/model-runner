package runtime

import (
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/errdefs"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

const (
	// DefaultContainerdSocket is the default containerd socket path.
	DefaultContainerdSocket = "/run/containerd/containerd.sock"
	// DefaultNamespace is the containerd namespace for dmrlet.
	DefaultNamespace = "dmrlet"
)

// Runner is the interface for container runtime operations.
type Runner interface {
	Run(ctx context.Context, spec ContainerSpec) error
	Stop(ctx context.Context, id string) error
	List(ctx context.Context) ([]ContainerInfo, error)
	Exists(ctx context.Context, id string) (bool, error)
	Close() error
}

// Runtime wraps containerd for container management.
type Runtime struct {
	client    *containerd.Client
	namespace string
	log       *logrus.Entry
}

// RuntimeOption configures the Runtime.
type RuntimeOption func(*runtimeOptions)

type runtimeOptions struct {
	socket    string
	namespace string
	logger    *logrus.Entry
}

// WithSocket sets the containerd socket path.
func WithSocket(socket string) RuntimeOption {
	return func(o *runtimeOptions) {
		o.socket = socket
	}
}

// WithNamespace sets the containerd namespace.
func WithNamespace(ns string) RuntimeOption {
	return func(o *runtimeOptions) {
		o.namespace = ns
	}
}

// WithRuntimeLogger sets the logger for the runtime.
func WithRuntimeLogger(logger *logrus.Entry) RuntimeOption {
	return func(o *runtimeOptions) {
		o.logger = logger
	}
}

// NewRuntime creates a new containerd runtime.
func NewRuntime(ctx context.Context, opts ...RuntimeOption) (*Runtime, error) {
	options := &runtimeOptions{
		socket:    getContainerdSocket(),
		namespace: DefaultNamespace,
		logger:    logrus.NewEntry(logrus.StandardLogger()),
	}
	for _, opt := range opts {
		opt(options)
	}

	client, err := containerd.New(options.socket)
	if err != nil {
		return nil, fmt.Errorf("connecting to containerd at %s: %w", options.socket, err)
	}

	return &Runtime{
		client:    client,
		namespace: options.namespace,
		log:       options.logger,
	}, nil
}

// getContainerdSocket returns the containerd socket path from environment or default.
func getContainerdSocket() string {
	if socket := os.Getenv("DMRLET_CONTAINERD_SOCK"); socket != "" {
		return socket
	}
	return DefaultContainerdSocket
}

// Close closes the runtime connection.
func (r *Runtime) Close() error {
	return r.client.Close()
}

// ContainerSpec defines the specification for creating a container.
type ContainerSpec struct {
	ID         string
	Image      string
	Command    []string
	Env        []string
	Mounts     []Mount
	GPU        *GPUInfo
	HostNet    bool
	WorkingDir string
}

// Mount defines a bind mount.
type Mount struct {
	Source      string
	Destination string
	ReadOnly    bool
}

// Run creates and starts a container.
func (r *Runtime) Run(ctx context.Context, spec ContainerSpec) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	r.log.Infof("Pulling image with ID sanitization")
	r.log.Debugf("Pulling image %s", sanitizeLogValue(spec.Image))

	// Pull the image
	image, err := r.client.Pull(ctx, spec.Image, containerd.WithPullUnpack)
	if err != nil {
		return fmt.Errorf("pulling image %s: %w", sanitizeLogValue(spec.Image), err)
	}

	r.log.Infof("Creating container with ID sanitization")
	r.log.Debugf("Creating container %s", sanitizeLogValue(spec.ID))

	// Build OCI spec options
	ociOpts := []oci.SpecOpts{
		oci.WithImageConfig(image),
	}

	if len(spec.Command) > 0 {
		cmd := resolveContainerCommand(spec.Command)
		ociOpts = append(ociOpts, oci.WithProcessArgs(cmd...))
	}

	if len(spec.Env) > 0 {
		ociOpts = append(ociOpts, oci.WithEnv(spec.Env))
	}

	if spec.HostNet {
		ociOpts = append(ociOpts, oci.WithHostNamespace(specs.NetworkNamespace))
		ociOpts = append(ociOpts, oci.WithHostHostsFile)
		ociOpts = append(ociOpts, oci.WithHostResolvconf)
	}

	// Add mounts
	for _, m := range spec.Mounts {
		opts := []string{"rbind"}
		if m.ReadOnly {
			opts = append(opts, "ro")
		}
		ociOpts = append(ociOpts, oci.WithMounts([]specs.Mount{
			{
				Type:        "bind",
				Source:      m.Source,
				Destination: m.Destination,
				Options:     opts,
			},
		}))
	}

	// Add GPU devices
	if spec.GPU != nil && spec.GPU.Type != "none" {
		for _, device := range spec.GPU.Devices {
			ociOpts = append(ociOpts, oci.WithDevices(device, device, "rwm"))
		}
	}

	// Create container
	container, err := r.client.NewContainer(
		ctx,
		spec.ID,
		containerd.WithImage(image),
		containerd.WithNewSnapshot(spec.ID+"-snapshot", image),
		containerd.WithNewSpec(ociOpts...),
		containerd.WithContainerLabels(map[string]string{
			"dmrlet.managed": "true",
		}),
	)
	if err != nil {
		return fmt.Errorf("creating container %s: %w", sanitizeLogValue(spec.ID), err)
	}

	r.log.Infof("Starting container with ID sanitization")
	r.log.Debugf("Starting container %s", sanitizeLogValue(spec.ID))

	// Create and start task
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		container.Delete(ctx, containerd.WithSnapshotCleanup)
		return fmt.Errorf("creating task for container %s: %w", sanitizeLogValue(container.ID()), err)
	}

	if err := task.Start(ctx); err != nil {
		task.Delete(ctx)
		container.Delete(ctx, containerd.WithSnapshotCleanup)
		return fmt.Errorf("starting task for container %s: %w", sanitizeLogValue(container.ID()), err)
	}

	r.log.Infof("Container started successfully with ID sanitization")
	r.log.Debugf("Container %s started successfully", sanitizeLogValue(spec.ID))
	return nil
}

// Stop stops and removes a container.
func (r *Runtime) Stop(ctx context.Context, id string) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	container, err := r.client.LoadContainer(ctx, id)
	if err != nil {
		return fmt.Errorf("loading container %s: %w", sanitizeLogValue(id), err)
	}

	task, err := container.Task(ctx, nil)
	if err == nil {
		// Kill the task
		if err := task.Kill(ctx, syscall.SIGTERM); err != nil {
			r.log.Warnf("Failed to send SIGTERM to container with ID sanitization")
			r.log.Debugf("Failed to send SIGTERM to container %s: %v", sanitizeLogValue(id), err)
		}

		// Wait for task to exit with timeout
		exitCh, err := task.Wait(ctx)
		if err == nil {
			select {
			case <-exitCh:
				// Task exited
			case <-time.After(10 * time.Second):
				// Force kill
				if err := task.Kill(ctx, syscall.SIGKILL); err != nil {
					r.log.Warnf("Failed to send SIGKILL to container with ID sanitization")
					r.log.Debugf("Failed to send SIGKILL to container %s: %v", sanitizeLogValue(id), err)
				}
			}
		}

		// Delete the task
		if _, err := task.Delete(ctx); err != nil {
			r.log.Warnf("Failed to delete task for container with ID sanitization")
			r.log.Debugf("Failed to delete task for container %s: %v", sanitizeLogValue(id), err)
		}
	}

	// Delete the container
	if err := container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
		return fmt.Errorf("deleting container %s: %w", id, err)
	}

	r.log.Infof("Container stopped and removed with ID sanitization")
	r.log.Debugf("Container %s stopped and removed", sanitizeLogValue(id))
	return nil
}

// List returns all dmrlet-managed containers.
func (r *Runtime) List(ctx context.Context) ([]ContainerInfo, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	containers, err := r.client.Containers(ctx, "labels.\"dmrlet.managed\"==true")
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	var result []ContainerInfo
	for _, c := range containers {
		info, err := r.getContainerInfo(ctx, c)
		if err != nil {
			sanitizedID := sanitizeLogValue(c.ID())
			r.log.Warnf("Failed to get info for container with ID sanitization")
			r.log.Debugf("Failed to get info for container %s: %v", sanitizedID, err)
			continue
		}
		result = append(result, info)
	}

	return result, nil
}

// ContainerInfo contains information about a running container.
type ContainerInfo struct {
	ID      string
	Image   string
	Status  string
	Labels  map[string]string
	Created time.Time
}

func (r *Runtime) getContainerInfo(ctx context.Context, c containerd.Container) (ContainerInfo, error) {
	info := ContainerInfo{
		ID: c.ID(),
	}

	cInfo, err := c.Info(ctx)
	if err != nil {
		return info, err
	}

	info.Image = cInfo.Image
	info.Labels = cInfo.Labels
	info.Created = cInfo.CreatedAt

	// Get task status
	task, err := c.Task(ctx, nil)
	if err != nil {
		info.Status = "stopped"
	} else {
		status, err := task.Status(ctx)
		if err != nil {
			info.Status = "unknown"
		} else {
			info.Status = string(status.Status)
		}
	}

	return info, nil
}

// Exists checks if a container exists.
func (r *Runtime) Exists(ctx context.Context, id string) (bool, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	_, err := r.client.LoadContainer(ctx, id)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to load container %s: %w", sanitizeLogValue(id), err)
	}
	return true, nil
}

// PullImage pulls an image from a registry.
func (r *Runtime) PullImage(ctx context.Context, image string) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	_, err := r.client.Pull(ctx, image, containerd.WithPullUnpack)
	if err != nil {
		return fmt.Errorf("pulling image %s: %w", sanitizeLogValue(image), err)
	}

	return nil
}

// resolveContainerCommand applies the "com.docker." prefix to the executable name
// in the command. Docker Model Runner container images ship binaries with this prefix
// (e.g. "com.docker.llama-server" instead of "llama-server").
func resolveContainerCommand(command []string) []string {
	if len(command) == 0 {
		return command
	}
	result := make([]string, len(command))
	copy(result, command)
	if !strings.HasPrefix(result[0], "/app/bin/com.docker.") {
		result[0] = "/app/bin/com.docker." + result[0]
	}
	return result
}

// sanitizeLogValue sanitizes user-provided values to prevent log injection
func sanitizeLogValue(value string) string {
	// Replace newline and carriage return characters to prevent log forging
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "\r", "\\r")
	return value
}

// Compile-time checks.
var (
	_ Runner = (*Runtime)(nil)
	_ v1.Platform
	_ containers.Container
)
