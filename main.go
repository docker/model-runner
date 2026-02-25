package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends/diffusers"
	"github.com/docker/model-runner/pkg/inference/backends/llamacpp"
	"github.com/docker/model-runner/pkg/inference/backends/sglang"
	"github.com/docker/model-runner/pkg/inference/config"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/logging"
	"github.com/docker/model-runner/pkg/metrics"
	"github.com/docker/model-runner/pkg/routing"
	modeltls "github.com/docker/model-runner/pkg/tls"
)

const (
	// DefaultTLSPort is the default TLS port for Moby
	DefaultTLSPort = "12444"
)

// initLogger creates the application logger based on LOG_LEVEL env var.
func initLogger() *slog.Logger {
	level := logging.ParseLevel(os.Getenv("LOG_LEVEL"))
	return logging.NewLogger(level)
}

var log = initLogger()

// exitFunc is used for Fatal-like exits; overridden in tests.
var exitFunc = func(code int) { os.Exit(code) }

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	sockName := os.Getenv("MODEL_RUNNER_SOCK")
	if sockName == "" {
		sockName = "model-runner.sock"
	}

	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		log.Error("Failed to get user home directory", "error", err)
		exitFunc(1)
	}

	modelPath := os.Getenv("MODELS_PATH")
	if modelPath == "" {
		modelPath = filepath.Join(userHomeDir, ".docker", "models")
	}

	_, disableServerUpdate := os.LookupEnv("DISABLE_SERVER_UPDATE")
	if disableServerUpdate {
		llamacpp.ShouldUpdateServerLock.Lock()
		llamacpp.ShouldUpdateServer = false
		llamacpp.ShouldUpdateServerLock.Unlock()
	}

	desiredServerVersion, ok := os.LookupEnv("LLAMA_SERVER_VERSION")
	if ok {
		llamacpp.SetDesiredServerVersion(desiredServerVersion)
	}

	llamaServerPath := os.Getenv("LLAMA_SERVER_PATH")
	if llamaServerPath == "" {
		llamaServerPath = "/Applications/Docker.app/Contents/Resources/model-runner/bin"
	}

	// Get optional custom paths for other backends
	vllmServerPath := os.Getenv("VLLM_SERVER_PATH")
	sglangServerPath := os.Getenv("SGLANG_SERVER_PATH")
	mlxServerPath := os.Getenv("MLX_SERVER_PATH")
	diffusersServerPath := os.Getenv("DIFFUSERS_SERVER_PATH")
	vllmMetalServerPath := os.Getenv("VLLM_METAL_SERVER_PATH")

	// Create a proxy-aware HTTP transport
	// Use a safe type assertion with fallback, and explicitly set Proxy to http.ProxyFromEnvironment
	var baseTransport *http.Transport
	if t, ok := http.DefaultTransport.(*http.Transport); ok {
		baseTransport = t.Clone()
	} else {
		baseTransport = &http.Transport{}
	}
	baseTransport.Proxy = http.ProxyFromEnvironment

	log.Info("LLAMA_SERVER_PATH", "path", llamaServerPath)
	if vllmServerPath != "" {
		log.Info("VLLM_SERVER_PATH", "path", vllmServerPath)
	}
	if sglangServerPath != "" {
		log.Info("SGLANG_SERVER_PATH", "path", sglangServerPath)
	}
	if mlxServerPath != "" {
		log.Info("MLX_SERVER_PATH", "path", mlxServerPath)
	}
	if vllmMetalServerPath != "" {
		log.Info("VLLM_METAL_SERVER_PATH", "path", vllmMetalServerPath)
	}

	// Create llama.cpp configuration from environment variables
	llamaCppConfig, err := createLlamaCppConfigFromEnv()
	if err != nil {
		log.Error("invalid LLAMA_ARGS", "error", err)
		exitFunc(1)
		return
	}

	updatedServerPath := func() string {
		wd, _ := os.Getwd()
		d := filepath.Join(wd, "updated-inference", "bin")
		_ = os.MkdirAll(d, 0o755)
		return d
	}()

	svc, err := routing.NewService(routing.ServiceConfig{
		Log: log,
		ClientConfig: models.ClientConfig{
			StoreRootPath: modelPath,
			Logger:        log.With("component", "model-manager"),
			Transport:     baseTransport,
		},
		Backends: append(
			routing.DefaultBackendDefs(routing.BackendsConfig{
				Log:                  log,
				LlamaCppVendoredPath: llamaServerPath,
				LlamaCppUpdatedPath:  updatedServerPath,
				LlamaCppConfig:       llamaCppConfig,
				IncludeMLX:           true,
				MLXPath:              mlxServerPath,
				IncludeVLLM:          includeVLLM,
				VLLMPath:             vllmServerPath,
				VLLMMetalPath:        vllmMetalServerPath,
			}),
			routing.BackendDef{Name: sglang.Name, Init: func(mm *models.Manager) (inference.Backend, error) {
				return sglang.New(log, mm, log.With("component", sglang.Name), nil, sglangServerPath)
			}},
			routing.BackendDef{Name: diffusers.Name, Init: func(mm *models.Manager) (inference.Backend, error) {
				return diffusers.New(log, mm, log.With("component", diffusers.Name), nil, diffusersServerPath)
			}},
		),
		OnBackendError: func(name string, err error) {
			log.Error("unable to initialize backend", "backend", name, "error", err)
			exitFunc(1)
		},
		DefaultBackendName: llamacpp.Name,
		HTTPClient:         http.DefaultClient,
		MetricsTracker: metrics.NewTracker(
			http.DefaultClient,
			log.With("component", "metrics"),
			"",
			false,
		),
		IncludeResponsesAPI: true,
		ExtraRoutes: func(r *routing.NormalizedServeMux, s *routing.Service) {
			// Root handler - only catches exact "/" requests
			r.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
				if req.URL.Path != "/" {
					http.NotFound(w, req)
					return
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("Docker Model Runner is running"))
			})

			// Version endpoint
			r.HandleFunc("/version", func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(map[string]string{"version": Version}); err != nil {
					log.Warn("failed to write version response", "error", err)
				}
			})

			// Metrics endpoint
			if os.Getenv("DISABLE_METRICS") != "1" {
				metricsHandler := metrics.NewAggregatedMetricsHandler(
					log.With("component", "metrics"),
					s.SchedulerHTTP,
				)
				r.Handle("/metrics", metricsHandler)
				log.Info("Metrics endpoint enabled at /metrics")
			} else {
				log.Info("Metrics endpoint disabled")
			}
		},
	})
	if err != nil {
		log.Error("failed to initialize service", "error", err)
		exitFunc(1)
	}

	server := &http.Server{
		Handler:           svc.Router,
		ReadHeaderTimeout: 10 * time.Second,
	}
	serverErrors := make(chan error, 1)

	// TLS server (optional)
	var tlsServer *http.Server
	tlsServerErrors := make(chan error, 1)

	// Check if we should use TCP port instead of Unix socket
	tcpPort := os.Getenv("MODEL_RUNNER_PORT")
	if tcpPort != "" {
		// Use TCP port
		addr := ":" + tcpPort
		log.Info("Listening on TCP port", "port", tcpPort)
		server.Addr = addr
		go func() {
			serverErrors <- server.ListenAndServe()
		}()
	} else {
		// Use Unix socket
		if err := os.Remove(sockName); err != nil {
			if !os.IsNotExist(err) {
				log.Error("Failed to remove existing socket", "error", err)
				exitFunc(1)
			}
		}
		ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: sockName, Net: "unix"})
		if err != nil {
			log.Error("Failed to listen on socket", "error", err)
			exitFunc(1)
		}
		go func() {
			serverErrors <- server.Serve(ln)
		}()
	}

	// Start TLS server if enabled
	if os.Getenv("MODEL_RUNNER_TLS_ENABLED") == "true" {
		tlsPort := os.Getenv("MODEL_RUNNER_TLS_PORT")
		if tlsPort == "" {
			tlsPort = DefaultTLSPort // Default TLS port for Moby
		}

		// Get certificate paths
		certPath := os.Getenv("MODEL_RUNNER_TLS_CERT")
		keyPath := os.Getenv("MODEL_RUNNER_TLS_KEY")

		// Auto-generate certificates if not provided and auto-cert is not disabled
		if certPath == "" || keyPath == "" {
			if os.Getenv("MODEL_RUNNER_TLS_AUTO_CERT") != "false" {
				log.Info("Auto-generating TLS certificates...")
				var err error
				certPath, keyPath, err = modeltls.EnsureCertificates("", "")
				if err != nil {
					log.Error("Failed to ensure TLS certificates", "error", err)
					exitFunc(1)
				}
				log.Info("Using TLS certificate", "cert", certPath)
				log.Info("Using TLS key", "key", keyPath)
			} else {
				log.Error("TLS enabled but no certificate provided and auto-cert is disabled")
				exitFunc(1)
			}
		}

		// Load TLS configuration
		tlsConfig, err := modeltls.LoadTLSConfig(certPath, keyPath)
		if err != nil {
			log.Error("Failed to load TLS configuration", "error", err)
			exitFunc(1)
		}

		tlsServer = &http.Server{
			Addr:              ":" + tlsPort,
			Handler:           svc.Router,
			TLSConfig:         tlsConfig,
			ReadHeaderTimeout: 10 * time.Second,
		}

		log.Info("Listening on TLS port", "port", tlsPort)
		go func() {
			// Use ListenAndServeTLS with empty strings since TLSConfig already has the certs
			ln, err := tls.Listen("tcp", tlsServer.Addr, tlsConfig)
			if err != nil {
				tlsServerErrors <- err
				return
			}
			tlsServerErrors <- tlsServer.Serve(ln)
		}()
	}

	schedulerErrors := make(chan error, 1)
	go func() {
		schedulerErrors <- svc.Scheduler.Run(ctx)
	}()

	var tlsServerErrorsChan <-chan error
	if os.Getenv("MODEL_RUNNER_TLS_ENABLED") == "true" {
		tlsServerErrorsChan = tlsServerErrors
	} else {
		// Use a nil channel which will block forever when TLS is disabled
		tlsServerErrorsChan = nil
	}

	select {
	case err := <-serverErrors:
		if err != nil {
			log.Error("Server error", "error", err)
		}
	case err := <-tlsServerErrorsChan:
		if err != nil {
			log.Error("TLS server error", "error", err)
		}
	case <-ctx.Done():
		log.Info("Shutdown signal received")
		log.Info("Shutting down the server")
		if err := server.Close(); err != nil {
			log.Error("Server shutdown error", "error", err)
		}
		if tlsServer != nil {
			log.Info("Shutting down the TLS server")
			if err := tlsServer.Close(); err != nil {
				log.Error("TLS server shutdown error", "error", err)
			}
		}
		log.Info("Waiting for the scheduler to stop")
		if err := <-schedulerErrors; err != nil {
			log.Error("Scheduler error", "error", err)
		}
	}
	log.Info("Docker Model Runner stopped")
}

// createLlamaCppConfigFromEnv creates a LlamaCppConfig from environment variables.
// Returns nil config (use defaults) when LLAMA_ARGS is unset, or an error if
// the args contain disallowed flags.
func createLlamaCppConfigFromEnv() (config.BackendConfig, error) {
	argsStr := os.Getenv("LLAMA_ARGS")
	if argsStr == "" {
		return nil, nil
	}

	args := splitArgs(argsStr)

	disallowedArgs := map[string]struct{}{
		"--model":      {},
		"--host":       {},
		"--embeddings": {},
		"--mmproj":     {},
	}
	for _, arg := range args {
		if _, found := disallowedArgs[arg]; found {
			return nil, fmt.Errorf("LLAMA_ARGS cannot override %s, which is controlled by the model runner", arg)
		}
	}

	log.Info("Using custom llama.cpp arguments", "args", args)
	return &llamacpp.Config{Args: args}, nil
}

// splitArgs splits a string into arguments, respecting quoted arguments
func splitArgs(s string) []string {
	var args []string
	var currentArg strings.Builder
	inQuotes := false

	for _, r := range s {
		switch {
		case r == '"' || r == '\'':
			inQuotes = !inQuotes
		case r == ' ' && !inQuotes:
			if currentArg.Len() > 0 {
				args = append(args, currentArg.String())
				currentArg.Reset()
			}
		default:
			currentArg.WriteRune(r)
		}
	}

	if currentArg.Len() > 0 {
		args = append(args, currentArg.String())
	}

	return args
}
