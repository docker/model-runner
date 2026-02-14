package inference

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/docker/model-runner/pkg/dmrlet/models"
	"github.com/docker/model-runner/pkg/dmrlet/network"
	"github.com/docker/model-runner/pkg/dmrlet/runtime"
	"github.com/sirupsen/logrus"
)

// Manager manages inference containers.
type Manager struct {
	store     *models.Store
	runtime   runtime.Runner
	ports     *network.PortAllocator
	health    *HealthChecker
	log       *logrus.Entry
	running   map[string]*RunningModel // containerID -> RunningModel
	runningMu sync.RWMutex
}

// RunningModel represents a running inference container.
type RunningModel struct {
	ContainerID string
	ModelRef    string
	Port        int
	Backend     models.Backend
	Endpoint    string
}

// ManagerOption configures the Manager.
type ManagerOption func(*managerOptions)

type managerOptions struct {
	logger *logrus.Entry
}

// WithManagerLogger sets the logger for the manager.
func WithManagerLogger(logger *logrus.Entry) ManagerOption {
	return func(o *managerOptions) {
		o.logger = logger
	}
}

// NewManager creates a new inference manager.
func NewManager(store *models.Store, rt runtime.Runner, opts ...ManagerOption) *Manager {
	options := &managerOptions{
		logger: logrus.NewEntry(logrus.StandardLogger()),
	}
	for _, opt := range opts {
		opt(options)
	}

	return &Manager{
		store:   store,
		runtime: rt,
		ports:   network.NewPortAllocator(),
		health:  NewHealthChecker(),
		log:     options.logger,
		running: make(map[string]*RunningModel),
	}
}

// ServeOptions configures a model serving request.
type ServeOptions struct {
	Port     int
	Backend  string
	GPU      bool
	Detach   bool
	Progress io.Writer
}

// Serve starts serving a model.
func (m *Manager) Serve(ctx context.Context, modelRef string, opts ServeOptions) (*RunningModel, error) {
	m.log.Infof("Serving model: %s", modelRef)

	// Ensure model is available
	if err := m.store.EnsureModel(ctx, modelRef, opts.Progress); err != nil {
		return nil, fmt.Errorf("ensuring model: %w", err)
	}

	// Get model bundle
	bundle, err := m.store.GetBundle(modelRef)
	if err != nil {
		return nil, fmt.Errorf("getting bundle: %w", err)
	}

	// Determine backend
	var backend models.Backend
	if opts.Backend != "" {
		backend = models.Backend(opts.Backend)
	} else {
		backend = models.DetectBackend(bundle)
	}
	m.log.Infof("Using backend: %s", backend)

	// Detect GPU
	var gpu *runtime.GPUInfo
	if opts.GPU {
		gpu = runtime.DetectGPU()
		if gpu.Type == "none" {
			m.log.Warnf("GPU requested but none detected")
		} else {
			m.log.Infof("Detected GPU: %s", gpu.Type)
		}
	}

	// Get container ID
	containerID := ContainerIDFromRef(modelRef)

	// Check if already running
	m.runningMu.RLock()
	if existing, exists := m.running[containerID]; exists {
		m.runningMu.RUnlock()
		return existing, nil
	}
	m.runningMu.RUnlock()

	// Check if container exists in runtime
	exists, err := m.runtime.Exists(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("checking container existence: %w", err)
	}
	if exists {
		// Stop existing container
		m.log.Infof("Stopping existing container: %s", containerID)
		if err := m.runtime.Stop(ctx, containerID); err != nil {
			m.log.Warnf("Failed to stop existing container: %v", err)
		}
	}

	// Allocate port
	port, err := m.ports.Allocate(containerID, opts.Port)
	if err != nil {
		return nil, fmt.Errorf("allocating port: %w", err)
	}
	m.log.Infof("Allocated port: %d", port)

	// Build container spec
	specBuilder := NewSpecBuilder(gpu)
	spec := specBuilder.Build(modelRef, bundle, backend, port)

	// Start container
	if err := m.runtime.Run(ctx, spec); err != nil {
		m.ports.Release(port)
		return nil, fmt.Errorf("starting container: %w", err)
	}

	// Wait for health
	m.log.Infof("Waiting for model to be ready...")
	if err := m.health.WaitForReady(ctx, port); err != nil {
		// Stop the container if health check fails
		m.runtime.Stop(ctx, containerID)
		m.ports.Release(port)
		return nil, fmt.Errorf("health check: %w", err)
	}

	running := &RunningModel{
		ContainerID: containerID,
		ModelRef:    modelRef,
		Port:        port,
		Backend:     backend,
		Endpoint:    fmt.Sprintf("http://localhost:%d/v1", port),
	}

	// Track running model
	m.runningMu.Lock()
	m.running[containerID] = running
	m.runningMu.Unlock()

	m.log.Infof("Model ready at %s", running.Endpoint)
	return running, nil
}

// Stop stops a running model.
func (m *Manager) Stop(ctx context.Context, modelRef string) error {
	containerID := ContainerIDFromRef(modelRef)

	m.runningMu.Lock()
	delete(m.running, containerID)
	m.runningMu.Unlock()

	// Release port
	m.ports.ReleaseByID(containerID)

	// Stop container
	if err := m.runtime.Stop(ctx, containerID); err != nil {
		return fmt.Errorf("stopping container: %w", err)
	}

	m.log.Infof("Stopped model: %s", modelRef)
	return nil
}

// List returns all running models.
func (m *Manager) List(ctx context.Context) ([]*RunningModel, error) {
	// Get containers from runtime
	containers, err := m.runtime.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	var result []*RunningModel

	m.runningMu.RLock()
	defer m.runningMu.RUnlock()

	for _, c := range containers {
		if running, exists := m.running[c.ID]; exists {
			// Check if still healthy
			if m.health.CheckHealth(running.Port) {
				result = append(result, running)
			}
		} else {
			// Container exists but not tracked - add with limited info
			port := m.ports.GetPort(c.ID)
			if port == 0 {
				continue
			}
			result = append(result, &RunningModel{
				ContainerID: c.ID,
				Port:        port,
				Endpoint:    fmt.Sprintf("http://localhost:%d/v1", port),
			})
		}
	}

	return result, nil
}

// Get returns a running model by reference.
func (m *Manager) Get(modelRef string) (*RunningModel, bool) {
	containerID := ContainerIDFromRef(modelRef)
	m.runningMu.RLock()
	defer m.runningMu.RUnlock()
	running, exists := m.running[containerID]
	return running, exists
}
