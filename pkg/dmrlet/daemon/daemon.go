// Package daemon provides the main dmrlet daemon orchestrator.
package daemon

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/docker/model-runner/pkg/dmrlet/autoscaler"
	"github.com/docker/model-runner/pkg/dmrlet/container"
	"github.com/docker/model-runner/pkg/dmrlet/gpu"
	"github.com/docker/model-runner/pkg/dmrlet/health"
	"github.com/docker/model-runner/pkg/dmrlet/logging"
	"github.com/docker/model-runner/pkg/dmrlet/service"
	"github.com/docker/model-runner/pkg/dmrlet/store"
)

const (
	defaultContainerdAddress = "/run/containerd/containerd.sock"
	basePort                 = 30000
)

// Config configures the daemon.
type Config struct {
	SocketPath        string
	ContainerdAddress string
	ModelStorePath    string
	BasePort          int
}

// DefaultConfig returns the default daemon configuration.
func DefaultConfig() Config {
	return Config{
		SocketPath:        "/var/run/dmrlet.sock",
		ContainerdAddress: defaultContainerdAddress,
		BasePort:          basePort,
	}
}

// Daemon is the main dmrlet daemon orchestrator.
type Daemon struct {
	config Config

	// Core components
	gpuInventory     *gpu.Inventory
	gpuAllocator     *gpu.Allocator
	containerManager *container.Manager
	serviceRegistry  *service.Registry
	healthChecker    health.Checker
	autoscaler       autoscaler.Scaler
	logAggregator    *logging.Aggregator
	modelStore       *store.Integration

	// API server
	apiServer *APIServer

	// State
	mu       sync.RWMutex
	running  atomic.Bool
	nextPort int
	models   map[string]*ModelDeployment

	// Shutdown
	cancel context.CancelFunc
}

// ModelDeployment represents a deployed model.
type ModelDeployment struct {
	Model      string
	Backend    container.Backend
	Replicas   int
	Containers []string // Container IDs
	GPUs       []int
	Endpoints  []string
	CreatedAt  time.Time
	Config     ServeConfig
}

// ServeConfig holds configuration for serving a model.
type ServeConfig struct {
	Model       string
	Backend     string
	GPUSpec     string
	Replicas    int
	ContextSize int
	GPUMemory   float64
	ExtraArgs   []string
	ExtraEnv    map[string]string
}

// New creates a new daemon.
func New(config Config) (*Daemon, error) {
	gpuInv := gpu.NewInventory()

	d := &Daemon{
		config:       config,
		gpuInventory: gpuInv,
		gpuAllocator: gpu.NewAllocator(gpuInv),
		nextPort:     config.BasePort,
		models:       make(map[string]*ModelDeployment),
	}

	return d, nil
}

// Start starts the daemon.
func (d *Daemon) Start(ctx context.Context) error {
	if d.running.Load() {
		return fmt.Errorf("daemon is already running")
	}

	ctx, d.cancel = context.WithCancel(ctx)

	// Initialize GPU inventory
	if err := d.gpuInventory.Refresh(); err != nil {
		// Non-fatal: we can run without GPUs
		fmt.Printf("Warning: failed to detect GPUs: %v\n", err)
	}

	// Initialize container manager
	var err error
	d.containerManager, err = container.NewManager(d.config.ContainerdAddress)
	if err != nil {
		return fmt.Errorf("failed to create container manager: %w", err)
	}

	if err := d.containerManager.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to containerd: %w", err)
	}

	// Initialize service registry
	d.serviceRegistry = service.NewRegistry()

	// Initialize health checker
	d.healthChecker = health.NewChecker(d.serviceRegistry, d.containerManager)
	go d.healthChecker.Run(ctx)

	// Initialize autoscaler
	d.autoscaler = autoscaler.NewScaler(d.serviceRegistry, d)

	// Initialize log aggregator
	d.logAggregator = logging.NewAggregator()

	// Initialize model store integration
	d.modelStore = store.NewIntegration(d.config.ModelStorePath)

	// Start API server
	d.apiServer = NewAPIServer(d, d.config.SocketPath)
	if err := d.apiServer.Start(); err != nil {
		return fmt.Errorf("failed to start API server: %w", err)
	}

	d.running.Store(true)

	return nil
}

// Stop stops the daemon.
func (d *Daemon) Stop(ctx context.Context) error {
	if !d.running.Load() {
		return nil
	}

	d.running.Store(false)

	// Cancel background goroutines
	if d.cancel != nil {
		d.cancel()
	}

	// Stop API server
	if d.apiServer != nil {
		if err := d.apiServer.Stop(); err != nil {
			fmt.Printf("Warning: failed to stop API server: %v\n", err)
		}
	}

	// Stop all containers
	d.mu.RLock()
	models := make([]*ModelDeployment, 0, len(d.models))
	for _, m := range d.models {
		models = append(models, m)
	}
	d.mu.RUnlock()

	for _, m := range models {
		if err := d.StopModel(ctx, m.Model); err != nil {
			fmt.Printf("Warning: failed to stop model %s: %v\n", m.Model, err)
		}
	}

	// Close container manager
	if d.containerManager != nil {
		if err := d.containerManager.Close(); err != nil {
			fmt.Printf("Warning: failed to close container manager: %v\n", err)
		}
	}

	return nil
}

// Serve starts serving a model.
func (d *Daemon) Serve(ctx context.Context, config ServeConfig) (*ModelDeployment, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Check if model is already deployed
	if _, ok := d.models[config.Model]; ok {
		return nil, fmt.Errorf("model %s is already deployed", config.Model)
	}

	// Resolve backend
	backend := container.BackendLlamaCpp
	if config.Backend != "" {
		var err error
		backend, err = container.ParseBackend(config.Backend)
		if err != nil {
			return nil, err
		}
	}

	// Get model path from store
	modelPath, err := d.modelStore.GetModelPath(config.Model)
	if err != nil {
		return nil, fmt.Errorf("model not found in store: %w", err)
	}

	// Parse GPU spec and allocate
	var gpuType gpu.GPUType
	var gpuIndices []int

	if config.GPUSpec != "" {
		strategy, indices, err := gpu.ParseGPUSpec(config.GPUSpec)
		if err != nil {
			return nil, fmt.Errorf("invalid GPU spec: %w", err)
		}

		allocation, err := d.gpuAllocator.Allocate(gpu.AllocationRequest{
			Assignee:   config.Model,
			Strategy:   strategy,
			GPUIndices: indices,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to allocate GPUs: %w", err)
		}

		if len(allocation) > 0 {
			gpuType = allocation[0].Type
			for _, g := range allocation {
				gpuIndices = append(gpuIndices, g.Index)
			}
		}
	} else {
		// Auto-detect: try to allocate all available GPUs
		allocation, err := d.gpuAllocator.Allocate(gpu.AllocationRequest{
			Assignee: config.Model,
			Strategy: gpu.StrategyAll,
		})
		if err == nil && len(allocation) > 0 {
			gpuType = allocation[0].Type
			for _, g := range allocation {
				gpuIndices = append(gpuIndices, g.Index)
			}
		}
	}

	// Build container spec
	specBuilder, err := container.NewSpecBuilder(backend)
	if err != nil {
		d.gpuAllocator.ReleaseByAssignee(config.Model)
		return nil, err
	}

	replicas := config.Replicas
	if replicas <= 0 {
		replicas = 1
	}

	// Validate replicas to prevent excessive memory allocation
	if replicas > 1000 {
		return nil, fmt.Errorf("too many replicas requested: %d (maximum: 1000)", replicas)
	}

	deployment := &ModelDeployment{
		Model:      config.Model,
		Backend:    backend,
		Replicas:   replicas,
		Containers: make([]string, 0, replicas),
		GPUs:       gpuIndices,
		Endpoints:  make([]string, 0, replicas),
		CreatedAt:  time.Now(),
		Config:     config,
	}

	// Create containers for each replica
	for i := 0; i < replicas; i++ {
		port := d.allocatePort()
		containerID := fmt.Sprintf("%s-%d-%s", sanitizeID(config.Model), i, generateRandomID())

		spec, err := specBuilder.Build(container.BuildOpts{
			Model:       config.Model,
			ModelPath:   modelPath,
			Port:        port,
			GPUType:     gpuType,
			GPUs:        gpuIndices,
			ContextSize: config.ContextSize,
			GPUMemory:   config.GPUMemory,
			ExtraArgs:   config.ExtraArgs,
			ExtraEnv:    config.ExtraEnv,
		})
		if err != nil {
			// Cleanup on failure
			d.cleanupDeployment(ctx, deployment)
			d.gpuAllocator.ReleaseByAssignee(config.Model)
			return nil, fmt.Errorf("failed to build container spec: %w", err)
		}

		// Pull image if needed
		if err := d.containerManager.PullImage(ctx, spec.Image); err != nil {
			// Try to continue - image might already exist
			fmt.Printf("Warning: failed to pull image %s: %v\n", spec.Image, err)
		}

		// Create container
		info, err := d.containerManager.Create(ctx, container.ContainerOpts{
			ID:         containerID,
			Model:      config.Model,
			Backend:    string(backend),
			Image:      spec.Image,
			ModelPath:  modelPath,
			Port:       port,
			GPUEnvVars: spec.Env,
			ExtraEnv:   config.ExtraEnv,
			Command:    spec.Command,
			Args:       spec.Args,
			Labels:     spec.Labels,
		})
		if err != nil {
			d.cleanupDeployment(ctx, deployment)
			d.gpuAllocator.ReleaseByAssignee(config.Model)
			return nil, fmt.Errorf("failed to create container: %w", err)
		}

		deployment.Containers = append(deployment.Containers, info.ID)
		deployment.Endpoints = append(deployment.Endpoints, info.Endpoint)

		// Register with service registry
		d.serviceRegistry.Register(service.Entry{
			Model:       config.Model,
			ContainerID: info.ID,
			Endpoint:    info.Endpoint,
			GPUs:        gpuIndices,
			Healthy:     true,
		})

		// Start log collection
		d.logAggregator.StartCollection(ctx, info.ID, d.containerManager)
	}

	d.models[config.Model] = deployment

	return deployment, nil
}

// StopModel stops all containers for a model.
func (d *Daemon) StopModel(ctx context.Context, model string) error {
	d.mu.Lock()
	deployment, ok := d.models[model]
	if !ok {
		d.mu.Unlock()
		return fmt.Errorf("model %s is not deployed", model)
	}
	delete(d.models, model)
	d.mu.Unlock()

	// Stop and remove containers
	for _, containerID := range deployment.Containers {
		// Unregister from service registry
		d.serviceRegistry.Unregister(containerID)

		// Stop log collection
		d.logAggregator.StopCollection(containerID)

		// Stop container
		if err := d.containerManager.Stop(ctx, containerID, 30*time.Second); err != nil {
			fmt.Printf("Warning: failed to stop container %s: %v\n", containerID, err)
		}

		// Remove container
		if err := d.containerManager.Remove(ctx, containerID); err != nil {
			fmt.Printf("Warning: failed to remove container %s: %v\n", containerID, err)
		}
	}

	// Release GPUs
	d.gpuAllocator.ReleaseByAssignee(model)

	return nil
}

// Scale scales a model to the specified number of replicas.
func (d *Daemon) Scale(ctx context.Context, model string, replicas int) error {
	d.mu.Lock()
	deployment, ok := d.models[model]
	if !ok {
		d.mu.Unlock()
		return fmt.Errorf("model %s is not deployed", model)
	}
	d.mu.Unlock()

	currentReplicas := len(deployment.Containers)

	if replicas == currentReplicas {
		return nil
	}

	if replicas > currentReplicas {
		// Scale up
		return d.scaleUp(ctx, model, replicas-currentReplicas)
	}

	// Scale down
	return d.scaleDown(ctx, model, currentReplicas-replicas)
}

func (d *Daemon) scaleUp(ctx context.Context, model string, count int) error {
	d.mu.Lock()
	deployment := d.models[model]
	d.mu.Unlock()

	for i := 0; i < count; i++ {
		// Create additional container
		config := deployment.Config

		// For new replicas, we need to allocate from remaining GPUs
		// or share existing GPUs based on strategy
		// For simplicity, new replicas share existing GPU allocation

		d.mu.Lock()
		port := d.allocatePort()
		idx := len(deployment.Containers)
		containerID := fmt.Sprintf("%s-%d-%s", sanitizeID(model), idx, generateRandomID())
		d.mu.Unlock()

		specBuilder, err := container.NewSpecBuilder(deployment.Backend)
		if err != nil {
			return fmt.Errorf("failed to create spec builder: %w", err)
		}

		modelPath, err := d.modelStore.GetModelPath(model)
		if err != nil {
			return fmt.Errorf("failed to get model path: %w", err)
		}

		var gpuType gpu.GPUType
		if len(deployment.GPUs) > 0 {
			gpuType = d.getGPUType(deployment.GPUs[0])
		}

		spec, err := specBuilder.Build(container.BuildOpts{
			Model:       model,
			ModelPath:   modelPath,
			Port:        port,
			GPUType:     gpuType,
			GPUs:        deployment.GPUs,
			ContextSize: config.ContextSize,
			GPUMemory:   config.GPUMemory,
			ExtraArgs:   config.ExtraArgs,
			ExtraEnv:    config.ExtraEnv,
		})
		if err != nil {
			return fmt.Errorf("failed to build container spec: %w", err)
		}

		info, err := d.containerManager.Create(ctx, container.ContainerOpts{
			ID:         containerID,
			Model:      model,
			Backend:    string(deployment.Backend),
			Image:      spec.Image,
			ModelPath:  modelPath,
			Port:       port,
			GPUEnvVars: spec.Env,
			Command:    spec.Command,
			Args:       spec.Args,
			Labels:     spec.Labels,
		})
		if err != nil {
			return fmt.Errorf("failed to create container: %w", err)
		}

		d.mu.Lock()
		deployment.Containers = append(deployment.Containers, info.ID)
		deployment.Endpoints = append(deployment.Endpoints, info.Endpoint)
		deployment.Replicas++
		d.mu.Unlock()

		d.serviceRegistry.Register(service.Entry{
			Model:       model,
			ContainerID: info.ID,
			Endpoint:    info.Endpoint,
			GPUs:        deployment.GPUs,
			Healthy:     true,
		})

		d.logAggregator.StartCollection(ctx, info.ID, d.containerManager)
	}

	return nil
}

func (d *Daemon) scaleDown(ctx context.Context, model string, count int) error {
	d.mu.Lock()
	deployment := d.models[model]

	if count > len(deployment.Containers) {
		count = len(deployment.Containers) - 1 // Keep at least one
	}

	// Remove containers from the end
	toRemove := deployment.Containers[len(deployment.Containers)-count:]
	deployment.Containers = deployment.Containers[:len(deployment.Containers)-count]
	deployment.Endpoints = deployment.Endpoints[:len(deployment.Endpoints)-count]
	deployment.Replicas -= count
	d.mu.Unlock()

	for _, containerID := range toRemove {
		d.serviceRegistry.Unregister(containerID)
		d.logAggregator.StopCollection(containerID)

		if err := d.containerManager.Stop(ctx, containerID, 30*time.Second); err != nil {
			fmt.Printf("Warning: failed to stop container %s: %v\n", containerID, err)
		}

		if err := d.containerManager.Remove(ctx, containerID); err != nil {
			fmt.Printf("Warning: failed to remove container %s: %v\n", containerID, err)
		}
	}

	return nil
}

// ListModels returns all deployed models.
func (d *Daemon) ListModels() []*ModelDeployment {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make([]*ModelDeployment, 0, len(d.models))
	for _, m := range d.models {
		result = append(result, m)
	}
	return result
}

// GetModel returns a specific model deployment.
func (d *Daemon) GetModel(model string) (*ModelDeployment, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	m, ok := d.models[model]
	return m, ok
}

// Status returns the daemon status.
func (d *Daemon) Status() *DaemonStatus {
	gpus := d.gpuInventory.All()

	return &DaemonStatus{
		Running: d.running.Load(),
		GPUs:    gpus,
		Models:  len(d.models),
		Socket:  d.config.SocketPath,
	}
}

// DaemonStatus holds daemon status information.
type DaemonStatus struct {
	Running bool
	GPUs    []gpu.GPU
	Models  int
	Socket  string
}

// GetLogs returns logs for a model.
func (d *Daemon) GetLogs(model string, lines int, follow bool) (<-chan logging.LogLine, error) {
	d.mu.RLock()
	deployment, ok := d.models[model]
	d.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("model %s is not deployed", model)
	}

	if len(deployment.Containers) == 0 {
		return nil, fmt.Errorf("no containers for model %s", model)
	}

	// Return logs from the first container
	return d.logAggregator.StreamLogs(context.Background(), deployment.Containers[0], lines, follow)
}

func (d *Daemon) allocatePort() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	port := d.nextPort
	d.nextPort++
	return port
}

func (d *Daemon) cleanupDeployment(ctx context.Context, deployment *ModelDeployment) {
	for _, containerID := range deployment.Containers {
		d.containerManager.Stop(ctx, containerID, 10*time.Second)
		d.containerManager.Remove(ctx, containerID)
		d.serviceRegistry.Unregister(containerID)
	}
}

func (d *Daemon) getGPUType(idx int) gpu.GPUType {
	g, ok := d.gpuInventory.Get(idx)
	if !ok {
		return gpu.GPUTypeUnknown
	}
	return g.Type
}

// GetServiceRegistry returns the service registry.
func (d *Daemon) GetServiceRegistry() *service.Registry {
	return d.serviceRegistry
}

// GetContainerManager returns the container manager.
func (d *Daemon) GetContainerManager() *container.Manager {
	return d.containerManager
}

func generateRandomID() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		// fallback to timestamp if random generation fails
		return fmt.Sprintf("%d", time.Now().UnixNano()%1000000)
	}
	return fmt.Sprintf("%06d", n.Int64())
}

func sanitizeID(s string) string {
	// Replace non-alphanumeric characters with dashes
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			result = append(result, c)
		} else {
			result = append(result, '-')
		}
	}
	return string(result)
}
