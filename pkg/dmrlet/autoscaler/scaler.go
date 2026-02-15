// Package autoscaler provides auto-scaling capabilities for dmrlet.
package autoscaler

// Scaler defines the interface for scaling models.
type Scaler interface {
	Scale(model string, replicas int) error
	GetReplicas(model string) (int, error)
}

// MockScaler implements the Scaler interface for testing.
type MockScaler struct{}

// Scale implements the Scaler interface.
func (m *MockScaler) Scale(model string, replicas int) error {
	return nil
}

// GetReplicas implements the Scaler interface.
func (m *MockScaler) GetReplicas(model string) (int, error) {
	return 1, nil
}

// NewScaler creates a new scaler.
func NewScaler(serviceRegistry interface{}, daemon interface{}) Scaler {
	return &MockScaler{}
}
