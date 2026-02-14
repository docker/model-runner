// Package network provides port allocation for dmrlet.
package network

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
)

const (
	// DefaultBasePort is the starting port for auto-allocation.
	DefaultBasePort = 30000
	// DefaultPortRangeSize is the number of ports available for allocation.
	DefaultPortRangeSize = 1000
)

// PortAllocator manages port allocation for inference containers.
type PortAllocator struct {
	basePort int
	maxPort  int
	used     map[int]string // port -> container ID
	mu       sync.Mutex
}

// NewPortAllocator creates a new port allocator.
func NewPortAllocator() *PortAllocator {
	basePort := DefaultBasePort
	if envPort := os.Getenv("DMRLET_BASE_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil && p > 0 && p < 65535 {
			basePort = p
		}
	}

	return &PortAllocator{
		basePort: basePort,
		maxPort:  basePort + DefaultPortRangeSize,
		used:     make(map[int]string),
	}
}

// Allocate finds and reserves an available port.
// If preferredPort is > 0, it tries to allocate that specific port.
func (a *PortAllocator) Allocate(containerID string, preferredPort int) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// If a preferred port is specified, try to use it
	if preferredPort > 0 {
		if err := a.checkPortAvailable(preferredPort); err != nil {
			return 0, fmt.Errorf("port %d: %w", preferredPort, err)
		}
		if owner, exists := a.used[preferredPort]; exists {
			return 0, fmt.Errorf("port %d already allocated to %s", preferredPort, owner)
		}
		a.used[preferredPort] = containerID
		return preferredPort, nil
	}

	// Auto-allocate from the port range
	for port := a.basePort; port < a.maxPort; port++ {
		if _, exists := a.used[port]; exists {
			continue
		}
		if err := a.checkPortAvailable(port); err != nil {
			continue
		}
		a.used[port] = containerID
		return port, nil
	}

	return 0, fmt.Errorf("no available ports in range %d-%d", a.basePort, a.maxPort-1)
}

// Release frees a previously allocated port.
func (a *PortAllocator) Release(port int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.used, port)
}

// ReleaseByID releases all ports allocated to a container ID.
func (a *PortAllocator) ReleaseByID(containerID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for port, id := range a.used {
		if id == containerID {
			delete(a.used, port)
		}
	}
}

// GetPort returns the port allocated to a container, or 0 if not found.
func (a *PortAllocator) GetPort(containerID string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	for port, id := range a.used {
		if id == containerID {
			return port
		}
	}
	return 0
}

// ListAllocations returns all current port allocations.
func (a *PortAllocator) ListAllocations() map[int]string {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := make(map[int]string, len(a.used))
	for port, id := range a.used {
		result[port] = id
	}
	return result
}

// checkPortAvailable checks if a port is available for binding.
func (a *PortAllocator) checkPortAvailable(port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("port in use: %w", err)
	}
	ln.Close()
	return nil
}

// IsPortAvailable checks if a specific port is available.
func IsPortAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}
