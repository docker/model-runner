// Package service provides service management utilities for dmrlet.
package service

// EndpointEntry represents an endpoint entry in the registry.
type EndpointEntry struct {
	Model    string
	Endpoint string
	GPUs     []int
	Healthy  bool
}

// Service represents a service managed by dmrlet.
type Service struct {
	Name string
}

// Entry represents a service entry in the registry.
type Entry struct {
	Model       string
	Endpoint    string
	GPUs        []int
	Healthy     bool
	ContainerID string
}

// Registry manages service registrations.
type Registry struct {
	// Add registry fields here
}

// NewRegistry creates a new service registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Register registers a service.
func (r *Registry) Register(entry Entry) error {
	// Implementation
	return nil
}

// Unregister unregisters a service.
func (r *Registry) Unregister(model string) error {
	// Implementation
	return nil
}

// ListModels returns the list of registered models.
func (r *Registry) ListModels() []string {
	// Implementation
	return []string{}
}

// GetByModel returns services for a specific model.
func (r *Registry) GetByModel(model string) []Entry {
	// Implementation
	return []Entry{}
}
