package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/model-runner/pkg/dmrlet/logging"
)

// APIServer provides HTTP API for CLI communication.
type APIServer struct {
	daemon   *Daemon
	socket   string
	listener net.Listener
	server   *http.Server
	mu       sync.Mutex
}

// NewAPIServer creates a new API server.
func NewAPIServer(daemon *Daemon, socket string) *APIServer {
	return &APIServer{
		daemon: daemon,
		socket: socket,
	}
}

// Start starts the API server.
func (s *APIServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove existing socket file
	if err := os.RemoveAll(s.socket); err != nil {
		return fmt.Errorf("failed to remove existing socket: %w", err)
	}

	// Create listener
	listener, err := net.Listen("unix", s.socket)
	if err != nil {
		return fmt.Errorf("failed to listen on socket %s: %w", s.socket, err)
	}
	s.listener = listener

	// Set permissions
	if err := os.Chmod(s.socket, 0660); err != nil {
		listener.Close()
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}

	// Create HTTP server
	mux := http.NewServeMux()
	s.registerHandlers(mux)

	s.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
	}

	// Start serving
	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Printf("API server error: %v\n", err)
		}
	}()

	return nil
}

// Stop stops the API server.
func (s *APIServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server != nil {
		if err := s.server.Shutdown(context.Background()); err != nil {
			return err
		}
	}

	if s.listener != nil {
		s.listener.Close()
	}

	os.RemoveAll(s.socket)
	return nil
}

func (s *APIServer) registerHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/v1/serve", s.handleServe)
	mux.HandleFunc("/v1/models", s.handleModels)
	mux.HandleFunc("/v1/models/", s.handleModel)
	mux.HandleFunc("/v1/scale", s.handleScale)
	mux.HandleFunc("/v1/stop", s.handleStop)
	mux.HandleFunc("/v1/logs/", s.handleLogs)
	mux.HandleFunc("/v1/status", s.handleStatus)
	mux.HandleFunc("/v1/health", s.handleHealth)
}

// ServeRequest is the request body for /v1/serve
type ServeRequest struct {
	Model       string            `json:"model"`
	Backend     string            `json:"backend,omitempty"`
	GPUSpec     string            `json:"gpu_spec,omitempty"`
	Replicas    int               `json:"replicas,omitempty"`
	ContextSize int               `json:"context_size,omitempty"`
	GPUMemory   float64           `json:"gpu_memory,omitempty"`
	ExtraArgs   []string          `json:"extra_args,omitempty"`
	ExtraEnv    map[string]string `json:"extra_env,omitempty"`
}

// ServeResponse is the response for /v1/serve
type ServeResponse struct {
	Model     string   `json:"model"`
	Backend   string   `json:"backend"`
	Replicas  int      `json:"replicas"`
	GPUs      []int    `json:"gpus"`
	Endpoints []string `json:"endpoints"`
}

func (s *APIServer) handleServe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ServeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if req.Model == "" {
		http.Error(w, "Model is required", http.StatusBadRequest)
		return
	}

	sc := ServeConfig(req)
	deployment, err := s.daemon.Serve(r.Context(), sc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := ServeResponse{
		Model:     deployment.Model,
		Backend:   string(deployment.Backend),
		Replicas:  deployment.Replicas,
		GPUs:      deployment.GPUs,
		Endpoints: deployment.Endpoints,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		fmt.Printf("Failed to encode JSON response: %v\n", err)
		return
	}
}

// ModelResponse is the response for a single model.
type ModelResponse struct {
	Model     string   `json:"model"`
	Backend   string   `json:"backend"`
	Replicas  int      `json:"replicas"`
	GPUs      []int    `json:"gpus"`
	Endpoints []string `json:"endpoints"`
	Status    string   `json:"status"`
}

// ModelsResponse is the response for /v1/models
type ModelsResponse struct {
	Models []ModelResponse `json:"models"`
}

func (s *APIServer) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	deployments := s.daemon.ListModels()
	models := make([]ModelResponse, 0, len(deployments))

	for _, d := range deployments {
		status := "running"
		// Check health from service registry
		entries := s.daemon.serviceRegistry.GetByModel(d.Model)
		allHealthy := true
		for _, e := range entries {
			if !e.Healthy {
				allHealthy = false
				break
			}
		}
		if !allHealthy {
			status = "degraded"
		}

		models = append(models, ModelResponse{
			Model:     d.Model,
			Backend:   string(d.Backend),
			Replicas:  d.Replicas,
			GPUs:      d.GPUs,
			Endpoints: d.Endpoints,
			Status:    status,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(ModelsResponse{Models: models}); err != nil {
		fmt.Printf("Failed to encode JSON response: %v\n", err)
		return
	}
}

func (s *APIServer) handleModel(w http.ResponseWriter, r *http.Request) {
	// Extract model name from path
	model := strings.TrimPrefix(r.URL.Path, "/v1/models/")
	if model == "" {
		http.Error(w, "Model name required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		deployment, ok := s.daemon.GetModel(model)
		if !ok {
			http.Error(w, "Model not found", http.StatusNotFound)
			return
		}

		resp := ModelResponse{
			Model:     deployment.Model,
			Backend:   string(deployment.Backend),
			Replicas:  deployment.Replicas,
			GPUs:      deployment.GPUs,
			Endpoints: deployment.Endpoints,
			Status:    "running",
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			fmt.Printf("Failed to encode JSON response: %v\n", err)
			return
		}

	case http.MethodDelete:
		if err := s.daemon.StopModel(r.Context(), model); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// ScaleRequest is the request body for /v1/scale
type ScaleRequest struct {
	Model    string `json:"model"`
	Replicas int    `json:"replicas"`
}

func (s *APIServer) handleScale(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ScaleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if req.Model == "" {
		http.Error(w, "Model is required", http.StatusBadRequest)
		return
	}

	if req.Replicas < 1 {
		http.Error(w, "Replicas must be at least 1", http.StatusBadRequest)
		return
	}

	if err := s.daemon.Scale(r.Context(), req.Model, req.Replicas); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	deployment, _ := s.daemon.GetModel(req.Model)
	resp := ModelResponse{
		Model:     deployment.Model,
		Backend:   string(deployment.Backend),
		Replicas:  deployment.Replicas,
		GPUs:      deployment.GPUs,
		Endpoints: deployment.Endpoints,
		Status:    "running",
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		fmt.Printf("Failed to encode JSON response: %v\n", err)
		return
	}
}

// StopRequest is the request body for /v1/stop
type StopRequest struct {
	Model string `json:"model,omitempty"`
	All   bool   `json:"all,omitempty"`
}

func (s *APIServer) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req StopRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if req.All {
		// Stop all models
		for _, d := range s.daemon.ListModels() {
			if err := s.daemon.StopModel(r.Context(), d.Model); err != nil {
				fmt.Printf("Warning: failed to stop model %s: %v\n", d.Model, err)
			}
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if req.Model == "" {
		http.Error(w, "Model is required", http.StatusBadRequest)
		return
	}

	if err := s.daemon.StopModel(r.Context(), req.Model); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *APIServer) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract model name from path
	model := strings.TrimPrefix(r.URL.Path, "/v1/logs/")
	if model == "" {
		http.Error(w, "Model name required", http.StatusBadRequest)
		return
	}

	// Parse query parameters
	lines := 100
	if l := r.URL.Query().Get("lines"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			lines = n
		}
	}

	follow := r.URL.Query().Get("follow") == "true"

	logChan, err := s.daemon.GetLogs(model, lines, follow)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Set headers for streaming
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Transfer-Encoding", "chunked")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	for line := range logChan {
		fmt.Fprintf(w, "[%s] %s\n", line.Timestamp.Format("2006-01-02 15:04:05"), line.Message)
		flusher.Flush()

		// Check if client disconnected
		select {
		case <-r.Context().Done():
			return
		default:
		}
	}
}

// StatusResponse is the response for /v1/status
type StatusResponse struct {
	Running bool        `json:"running"`
	GPUs    []GPUStatus `json:"gpus"`
	Models  int         `json:"models"`
	Socket  string      `json:"socket"`
}

// GPUStatus holds GPU status information.
type GPUStatus struct {
	Index      int    `json:"index"`
	Type       string `json:"type"`
	Name       string `json:"name"`
	MemoryMB   int64  `json:"memory_mb"`
	InUse      bool   `json:"in_use"`
	AssignedTo string `json:"assigned_to,omitempty"`
}

func (s *APIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := s.daemon.Status()

	gpus := make([]GPUStatus, 0, len(status.GPUs))
	for _, g := range status.GPUs {
		gpus = append(gpus, GPUStatus{
			Index:      g.Index,
			Type:       string(g.Type),
			Name:       g.Name,
			MemoryMB:   int64(g.MemoryMB),
			InUse:      g.InUse,
			AssignedTo: g.AssignedTo,
		})
	}

	resp := StatusResponse{
		Running: status.Running,
		GPUs:    gpus,
		Models:  status.Models,
		Socket:  status.Socket,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		fmt.Printf("Failed to encode JSON response: %v\n", err)
		return
	}
}

func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		fmt.Printf("Failed to encode JSON response: %v\n", err)
		return
	}
}

// Client provides a client for the daemon API.
type Client struct {
	socket     string
	httpClient *http.Client
}

// NewClient creates a new daemon client.
func NewClient(socket string) *Client {
	return &Client{
		socket: socket,
		httpClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", socket)
				},
			},
		},
	}
}

// Serve starts serving a model.
func (c *Client) Serve(ctx context.Context, req ServeRequest) (*ServeResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/v1/serve", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("daemon error: %s", string(body))
	}

	var result ServeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// List lists all models.
func (c *Client) List(ctx context.Context) (*ModelsResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/v1/models", http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("daemon error: %s", string(body))
	}

	var result ModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// Scale scales a model.
func (c *Client) Scale(ctx context.Context, model string, replicas int) (*ModelResponse, error) {
	body, err := json.Marshal(ScaleRequest{Model: model, Replicas: replicas})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal scale request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/v1/scale", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("daemon error: %s", string(body))
	}

	var result ModelResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// Stop stops a model or all models.
func (c *Client) Stop(ctx context.Context, model string, all bool) error {
	body, err := json.Marshal(StopRequest{Model: model, All: all})
	if err != nil {
		return fmt.Errorf("failed to marshal stop request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/v1/stop", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("daemon error: %s", string(body))
	}

	return nil
}

// GetStatus gets daemon status.
func (c *Client) GetStatus(ctx context.Context) (*StatusResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/v1/status", http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("daemon error: %s", string(body))
	}

	var result StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// StreamLogs streams logs for a model.
func (c *Client) StreamLogs(ctx context.Context, model string, lines int, follow bool) (<-chan logging.LogLine, error) {
	url := fmt.Sprintf("http://unix/v1/logs/%s?lines=%d", model, lines)
	if follow {
		url += "&follow=true"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("daemon error: %s", string(body))
	}

	ch := make(chan logging.LogLine)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			case ch <- logging.LogLine{Message: scanner.Text() + "\n"}:
			}
		}
	}()

	return ch, nil
}
