package scheduling

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/sirupsen/logrus"
)

// mockBackend is a minimal backend implementation for testing
type mockBackend struct {
	name                  string
	requiredMemory        inference.RequiredMemory
	usesExternalModelMgmt bool
}

func (m *mockBackend) Name() string {
	return m.name
}

func (m *mockBackend) Install(ctx context.Context, httpClient *http.Client) error {
	return nil
}

func (m *mockBackend) Run(ctx context.Context, socket, model string, modelRef string, mode inference.BackendMode, config *inference.BackendConfiguration) error {
	return nil
}

func (m *mockBackend) Status() string {
	return "mock"
}

func (m *mockBackend) GetDiskUsage() (int64, error) {
	return 0, nil
}

func (m *mockBackend) GetRequiredMemoryForModel(ctx context.Context, model string, config *inference.BackendConfiguration) (inference.RequiredMemory, error) {
	return m.requiredMemory, nil
}

func (m *mockBackend) UsesExternalModelManagement() bool {
	return m.usesExternalModelMgmt
}

// fastFailBackend is a backend that immediately fails on Run to short-circuit wait()
type fastFailBackend struct{ mockBackend }

func (b *fastFailBackend) Run(ctx context.Context, socket, model string, modelRef string, mode inference.BackendMode, config *inference.BackendConfiguration) error {
	return errors.New("boom")
}

// createTestLogger creates a logger for testing
func createTestLogger() *logrus.Entry {
	log := logrus.New()
	log.SetOutput(io.Discard)
	return logrus.NewEntry(log)
}

// Test memory size constants
const (
	GB = 1024 * 1024 * 1024
)

// createDefunctMockRunner creates a mock runner with a closed done channel,
// simulating a defunct (crashed/terminated) runner for testing
func createDefunctMockRunner(log *logrus.Entry, backend inference.Backend) *runner {
	defunctRunnerDone := make(chan struct{})
	_, defunctRunnerCancel := context.WithCancel(context.Background())

	// Create minimal HTTP client and transport to avoid nil pointer errors
	transport := &http.Transport{}
	client := &http.Client{Transport: transport}

	defunctRunner := &runner{
		log:            log,
		backend:        backend,
		model:          "model1",
		mode:           inference.BackendModeCompletion,
		cancel:         defunctRunnerCancel,
		done:           defunctRunnerDone,
		transport:      transport,
		client:         client,
		proxyLog:       io.NopCloser(nil),
		openAIRecorder: nil,
	}

	// Close the done channel to mark it as defunct
	close(defunctRunnerDone)

	return defunctRunner
}

// createAliveTerminableMockRunner creates a mock runner with an open done channel
// (i.e., not defunct) that will close when cancel is invoked, so terminate() returns.
func createAliveTerminableMockRunner(log *logrus.Entry, backend inference.Backend) *runner {
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	// Create minimal HTTP client and transport to avoid nil pointer errors
	transport := &http.Transport{}
	client := &http.Client{Transport: transport}

	// Close done when cancel is called
	go func() {
		<-runCtx.Done()
		close(done)
	}()

	return &runner{
		log:            log,
		backend:        backend,
		model:          "modelX",
		mode:           inference.BackendModeCompletion,
		cancel:         cancel,
		done:           done,
		transport:      transport,
		client:         client,
		proxyLog:       io.NopCloser(nil),
		openAIRecorder: nil,
	}
}

// TestMakeRunnerKey tests that runner keys are created correctly
func TestMakeRunnerKey(t *testing.T) {
	tests := []struct {
		name         string
		backend      string
		modelID      string
		draftModelID string
		mode         inference.BackendMode
	}{
		{
			name:         "completion mode without draft",
			backend:      "llama.cpp",
			modelID:      "model123",
			draftModelID: "",
			mode:         inference.BackendModeCompletion,
		},
		{
			name:         "completion mode with draft",
			backend:      "llama.cpp",
			modelID:      "model123",
			draftModelID: "draft456",
			mode:         inference.BackendModeCompletion,
		},
		{
			name:         "embedding mode",
			backend:      "llama.cpp",
			modelID:      "model123",
			draftModelID: "",
			mode:         inference.BackendModeEmbedding,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := makeRunnerKey(tt.backend, tt.modelID, tt.draftModelID, tt.mode)

			if key.backend != tt.backend {
				t.Errorf("Expected backend %q, got %q", tt.backend, key.backend)
			}
			if key.modelID != tt.modelID {
				t.Errorf("Expected modelID %q, got %q", tt.modelID, key.modelID)
			}
			if key.draftModelID != tt.draftModelID {
				t.Errorf("Expected draftModelID %q, got %q", tt.draftModelID, key.draftModelID)
			}
			if key.mode != tt.mode {
				t.Errorf("Expected mode %v, got %v", tt.mode, key.mode)
			}
		})
	}
}

// TestMakeConfigKey tests that config keys exclude draft model ID
func TestMakeConfigKey(t *testing.T) {
	backend := "llama.cpp"
	modelID := "model123"
	mode := inference.BackendModeCompletion

	key := makeConfigKey(backend, modelID, mode)

	if key.backend != backend {
		t.Errorf("Expected backend %q, got %q", backend, key.backend)
	}
	if key.modelID != modelID {
		t.Errorf("Expected modelID %q, got %q", modelID, key.modelID)
	}
	if key.draftModelID != "" {
		t.Errorf("Expected empty draftModelID for config key, got %q", key.draftModelID)
	}
	if key.mode != mode {
		t.Errorf("Expected mode %v, got %v", mode, key.mode)
	}
}

// TestStopAndDrainTimer tests the timer draining utility
func TestStopAndDrainTimer(t *testing.T) {
	// Test with a timer that has fired
	timer1 := time.NewTimer(1 * time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	stopAndDrainTimer(timer1)

	// Test with a timer that hasn't fired
	timer2 := time.NewTimer(1 * time.Hour)
	stopAndDrainTimer(timer2)

	// Both should complete without blocking
}

// TestDefunctRunnerEvictionTriggersRetry tests that when a defunct runner is evicted
// during load(), the loop properly continues to retry slot allocation instead of
// waiting indefinitely.
func TestDefunctRunnerEvictionTriggersRetry(t *testing.T) {
	log := createTestLogger()

	// Create a backend that fails fast on Run and requires 1GB RAM, 1GB VRAM
	backend := &fastFailBackend{mockBackend: mockBackend{
		name: "test-backend",
		requiredMemory: inference.RequiredMemory{
			RAM:  1 * GB,
			VRAM: 1 * GB,
		},
	}}

	// Create the loader with minimal dependencies (nil model manager is fine for this test)
	backends := map[string]inference.Backend{"test-backend": backend}
	loader := newLoader(log, backends, nil, nil)

	// Enable loads directly under the lock (no background run loop needed)
	if !loader.lock(context.Background()) {
		t.Fatal("Failed to acquire loader lock to enable loads")
	}
	loader.loadsEnabled = true
	loader.unlock()

	// Set up a defunct runner in the loader's state to simulate an existing crashed runner
	if !loader.lock(context.Background()) {
		t.Fatal("Failed to acquire loader lock")
	}

	defunctRunner := createDefunctMockRunner(log, backend)

	// Register the defunct runner in slot 0
	slot := 0
	loader.slots[slot] = defunctRunner
	loader.runners[makeRunnerKey("test-backend", "model1", "", inference.BackendModeCompletion)] = runnerInfo{
		slot:     slot,
		modelRef: "model1:latest",
	}
	loader.references[slot] = 0 // Mark as unused (so it can be evicted)
	loader.timestamps[slot] = time.Now()

	loader.unlock()

	// Attempt to load - with fastFail backend, this should return quickly after eviction+retry
	_, err := loader.load(context.Background(), "test-backend", "model1", "model1:latest", inference.BackendModeCompletion)

	// We expect an error (backend fails fast), but not a timeout/hang
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("load() timed out - eviction likely did not trigger retry")
	}
	if err == nil {
		t.Log("Unexpected success; should never happen with fastFail backend")
	}
}

// TestUnusedRunnerEvictionTriggersRetry tests that when an unused (non-defunct)
// runner is evicted during load(), the loop properly continues to retry slot
// allocation instead of waiting indefinitely.
func TestUnusedRunnerEvictionTriggersRetry(t *testing.T) {
	log := createTestLogger()

	// Create a backend that fails fast on Run and requires 1GB RAM, 1GB VRAM
	backend := &fastFailBackend{mockBackend: mockBackend{
		name: "test-backend",
		requiredMemory: inference.RequiredMemory{
			RAM:  1 * GB,
			VRAM: 1 * GB,
		},
	}}

	backends := map[string]inference.Backend{"test-backend": backend}
	loader := newLoader(log, backends, nil, nil)

	// Enable loads directly
	if !loader.lock(context.Background()) {
		t.Fatal("Failed to acquire loader lock to enable loads")
	}
	loader.loadsEnabled = true
	loader.unlock()

	// Install an unused, alive runner under a different model key occupying all memory
	if !loader.lock(context.Background()) {
		t.Fatal("Failed to acquire loader lock")
	}

	aliveRunner := createAliveTerminableMockRunner(log, backend)
	slot := 0
	loader.slots[slot] = aliveRunner
	loader.runners[makeRunnerKey("test-backend", "modelX", "", inference.BackendModeCompletion)] = runnerInfo{
		slot:     slot,
		modelRef: "modelX:latest",
	}
	loader.references[slot] = 0 // unused
	loader.timestamps[slot] = time.Now()

	loader.unlock()

	// Attempt to load a different model; eviction should occur and loop should retry immediately
	_, err := loader.load(context.Background(), "test-backend", "model1", "model1:latest", inference.BackendModeCompletion)

	if errors.Is(err, context.DeadlineExceeded) {
		t.Error("load() timed out - eviction of unused runner did not trigger retry")
	}
	if err == nil {
		t.Error("Unexpected success; acceptable but unusual with fastFail backend")
	}
}
