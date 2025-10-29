package metrics

import (
	"io"
	"net"
	"net/http"
	"time"

	"github.com/docker/model-runner/pkg/logging"
)

// SchedulerMetricsHandler handles metrics requests by finding active llama.cpp runners
type SchedulerMetricsHandler struct {
	log       logging.Logger
	scheduler SchedulerInterface
}

// SchedulerInterface defines the methods we need from the scheduler
type SchedulerInterface interface {
	GetRunningBackends(w http.ResponseWriter, r *http.Request)
	GetLlamaCppSocket() (string, error)
	GetAllActiveRunners() []ActiveRunner
}

// ActiveRunner contains information about an active runner
type ActiveRunner struct {
	BackendName string
	ModelName   string
	Mode        string
	Socket      string
}

// ServeHTTP implements http.Handler for metrics proxying via scheduler
func (h *SchedulerMetricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	// Get the socket path for the active llama.cpp runner
	socket, err := h.scheduler.GetLlamaCppSocket()
	if err != nil {
		h.log.Errorf("Failed to get llama.cpp socket: %v", err)
		http.Error(w, "Metrics endpoint not available", http.StatusServiceUnavailable)
		return
	}

	// Create HTTP client for Unix socket communication
	client := &http.Client{
		Transport: &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				return net.DialTimeout("unix", socket, 5*time.Second)
			},
		},
		Timeout: 10 * time.Second,
	}

	// Create request to the backend metrics endpoint
	req, err := http.NewRequestWithContext(r.Context(), "GET", "http://unix/metrics", nil)
	if err != nil {
		h.log.Errorf("Failed to create metrics request: %v", err)
		http.Error(w, "Failed to create metrics request", http.StatusInternalServerError)
		return
	}

	// Forward relevant headers
	for key, values := range r.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// Make the request to the backend
	resp, err := client.Do(req)
	if err != nil {
		h.log.Errorf("Failed to fetch metrics from backend: %v", err)
		http.Error(w, "Backend metrics unavailable", http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Set status code
	w.WriteHeader(resp.StatusCode)

	// Copy response body
	if _, err := io.Copy(w, resp.Body); err != nil {
		h.log.Errorf("Failed to copy metrics response: %v", err)
		return
	}

	h.log.Debugf("Successfully proxied metrics request")
}
