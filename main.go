package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
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
	"github.com/docker/model-runner/pkg/metrics"
	"github.com/docker/model-runner/pkg/routing"
	modeltls "github.com/docker/model-runner/pkg/tls"
	"github.com/sirupsen/logrus"
)

const (
	// DefaultTLSPort is the default TLS port for Moby
	DefaultTLSPort = "12444"
)

var log = logrus.New()

// Log is the logger used by the application, exported for testing purposes.
var Log = log

// testLog is a test-override logger used by createLlamaCppConfigFromEnv.
var testLog = log

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	sockName := os.Getenv("MODEL_RUNNER_SOCK")
	if sockName == "" {
		sockName = "model-runner.sock"
	}

	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to get user home directory: %v", err)
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

	log.Infof("LLAMA_SERVER_PATH: %s", llamaServerPath)
	if vllmServerPath != "" {
		log.Infof("VLLM_SERVER_PATH: %s", vllmServerPath)
	}
	if sglangServerPath != "" {
		log.Infof("SGLANG_SERVER_PATH: %s", sglangServerPath)
	}
	if mlxServerPath != "" {
		log.Infof("MLX_SERVER_PATH: %s", mlxServerPath)
	}
	if vllmMetalServerPath != "" {
		log.Infof("VLLM_METAL_SERVER_PATH: %s", vllmMetalServerPath)
	}

	// Create llama.cpp configuration from environment variables
	llamaCppConfig := createLlamaCppConfigFromEnv()

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
			Logger:        log.WithFields(logrus.Fields{"component": "model-manager"}),
			Transport:     baseTransport,
		},
		Backends: append(append(
			routing.DefaultBackendDefs(routing.BackendsConfig{
				Log:                  log,
				LlamaCppVendoredPath: llamaServerPath,
				LlamaCppUpdatedPath:  updatedServerPath,
				LlamaCppConfig:       llamaCppConfig,
				IncludeMLX:           true,
				MLXPath:              mlxServerPath,
			}),
			routing.BackendDef{Name: sglang.Name, Init: func(mm *models.Manager) (inference.Backend, error) {
				return sglang.New(log, mm, log.WithFields(logrus.Fields{"component": sglang.Name}), nil, sglangServerPath)
			}},
			routing.BackendDef{Name: diffusers.Name, Init: func(mm *models.Manager) (inference.Backend, error) {
				return diffusers.New(log, mm, log.WithFields(logrus.Fields{"component": diffusers.Name}), nil, diffusersServerPath)
			}},
		), vllmBackendDefs(log, vllmServerPath)...),
		OnBackendError: func(name string, err error) {
			log.Fatalf("unable to initialize %s backend: %v", name, err)
		},
		DefaultBackendName:  llamacpp.Name,
		VLLMMetalServerPath: vllmMetalServerPath,
		HTTPClient:          http.DefaultClient,
		MetricsTracker: metrics.NewTracker(
			http.DefaultClient,
			log.WithField("component", "metrics"),
			"",
			false,
		),
		IncludeResponsesAPI: true,
		ExtraRoutes: func(r *routing.NormalizedServeMux, s *routing.Service) {
			// Root handler – only catches exact "/" requests
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
					log.Warnf("failed to write version response: %v", err)
				}
			})

			// Metrics endpoint
			if os.Getenv("DISABLE_METRICS") != "1" {
				metricsHandler := metrics.NewAggregatedMetricsHandler(
					log.WithField("component", "metrics"),
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
		log.Fatalf("failed to initialize service: %v", err)
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
		log.Infof("Listening on TCP port %s", tcpPort)
		server.Addr = addr
		go func() {
			serverErrors <- server.ListenAndServe()
		}()
	} else {
		// Use Unix socket
		if err := os.Remove(sockName); err != nil {
			if !os.IsNotExist(err) {
				log.Fatalf("Failed to remove existing socket: %v", err)
			}
		}
		ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: sockName, Net: "unix"})
		if err != nil {
			log.Fatalf("Failed to listen on socket: %v", err)
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
					log.Fatalf("Failed to ensure TLS certificates: %v", err)
				}
				log.Infof("Using TLS certificate: %s", certPath)
				log.Infof("Using TLS key: %s", keyPath)
			} else {
				log.Fatal("TLS enabled but no certificate provided and auto-cert is disabled")
			}
		}

		// Load TLS configuration
		tlsConfig, err := modeltls.LoadTLSConfig(certPath, keyPath)
		if err != nil {
			log.Fatalf("Failed to load TLS configuration: %v", err)
		}

		tlsServer = &http.Server{
			Addr:              ":" + tlsPort,
			Handler:           svc.Router,
			TLSConfig:         tlsConfig,
			ReadHeaderTimeout: 10 * time.Second,
		}

		log.Infof("Listening on TLS port %s", tlsPort)
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
			log.Errorf("Server error: %v", err)
		}
	case err := <-tlsServerErrorsChan:
		if err != nil {
			log.Errorf("TLS server error: %v", err)
		}
	case <-ctx.Done():
		log.Infoln("Shutdown signal received")
		log.Infoln("Shutting down the server")
		if err := server.Close(); err != nil {
			log.Errorf("Server shutdown error: %v", err)
		}
		if tlsServer != nil {
			log.Infoln("Shutting down the TLS server")
			if err := tlsServer.Close(); err != nil {
				log.Errorf("TLS server shutdown error: %v", err)
			}
		}
		log.Infoln("Waiting for the scheduler to stop")
		if err := <-schedulerErrors; err != nil {
			log.Errorf("Scheduler error: %v", err)
		}
	}
	log.Infoln("Docker Model Runner stopped")
}

// createLlamaCppConfigFromEnv creates a LlamaCppConfig from environment variables
func createLlamaCppConfigFromEnv() config.BackendConfig {
	// Check if any configuration environment variables are set
	argsStr := os.Getenv("LLAMA_ARGS")

	// If no environment variables are set, use default configuration
	if argsStr == "" {
		return nil // nil will cause the backend to use its default configuration
	}

	// Split the string by spaces, respecting quoted arguments
	args := splitArgs(argsStr)

	// Check for disallowed arguments
	disallowedArgs := []string{"--model", "--host", "--embeddings", "--mmproj"}
	for _, arg := range args {
		for _, disallowed := range disallowedArgs {
			if arg == disallowed {
				testLog.Fatalf("LLAMA_ARGS cannot override the %s argument as it is controlled by the model runner", disallowed)
			}
		}
	}

	testLog.Infof("Using custom arguments: %v", args)
	return &llamacpp.Config{
		Args: args,
	}
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
