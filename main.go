package main

import (
	"fmt"
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/docker/model-runner/pkg/anthropic"
	"github.com/docker/model-runner/pkg/logging"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends/diffusers"
	"github.com/docker/model-runner/pkg/inference/backends/llamacpp"
	"github.com/docker/model-runner/pkg/inference/backends/mlx"
	"github.com/docker/model-runner/pkg/inference/backends/sglang"
	"github.com/docker/model-runner/pkg/inference/backends/vllm"
	"github.com/docker/model-runner/pkg/inference/backends/vllmmetal"
	"github.com/docker/model-runner/pkg/inference/config"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/inference/platform"
	"github.com/docker/model-runner/pkg/inference/scheduling"
	"github.com/docker/model-runner/pkg/metrics"
	"github.com/docker/model-runner/pkg/middleware"
	"github.com/docker/model-runner/pkg/ollama"
	"github.com/docker/model-runner/pkg/responses"
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
		log.Error(fmt.Sprintf("Failed to get user home directory: %v", err))
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

	clientConfig := models.ClientConfig{
		StoreRootPath: modelPath,
		Logger:        log.With("component", "model-manager"),
		Transport:     baseTransport,
	}
	modelManager := models.NewManager(log.With("component", "model-manager"), clientConfig)
	modelHandler := models.NewHTTPHandler(
		log,
		modelManager,
		nil,
	)
	log.Info(fmt.Sprintf("LLAMA_SERVER_PATH: %s", llamaServerPath))
	if vllmServerPath != "" {
		log.Info(fmt.Sprintf("VLLM_SERVER_PATH: %s", vllmServerPath))
	}
	if sglangServerPath != "" {
		log.Info(fmt.Sprintf("SGLANG_SERVER_PATH: %s", sglangServerPath))
	}
	if mlxServerPath != "" {
		log.Info(fmt.Sprintf("MLX_SERVER_PATH: %s", mlxServerPath))
	}
	if vllmMetalServerPath != "" {
		log.Info(fmt.Sprintf("VLLM_METAL_SERVER_PATH: %s", vllmMetalServerPath))
	}

	// Create llama.cpp configuration from environment variables
	llamaCppConfig := createLlamaCppConfigFromEnv()

	llamaCppBackend, err := llamacpp.New(
		log,
		modelManager,
		log.With("component", llamacpp.Name),
		llamaServerPath,
		func() string {
			wd, _ := os.Getwd()
			d := filepath.Join(wd, "updated-inference", "bin")
			_ = os.MkdirAll(d, 0o755)
			return d
		}(),
		llamaCppConfig,
	)
	if err != nil {
		log.Error(fmt.Sprintf("unable to initialize %s backend: %v", llamacpp.Name, err))
	}

	vllmBackend, err := initVLLMBackend(log, modelManager, vllmServerPath)
	if err != nil {
		log.Error(fmt.Sprintf("unable to initialize %s backend: %v", vllm.Name, err))
	}

	mlxBackend, err := mlx.New(
		log,
		modelManager,
		log.With("component", mlx.Name),
		nil,
		mlxServerPath,
	)
	if err != nil {
		log.Error(fmt.Sprintf("unable to initialize %s backend: %v", mlx.Name, err))
	}

	sglangBackend, err := sglang.New(
		log,
		modelManager,
		log.With("component", sglang.Name),
		nil,
		sglangServerPath,
	)
	if err != nil {
		log.Error(fmt.Sprintf("unable to initialize %s backend: %v", sglang.Name, err))
	}

	diffusersBackend, err := diffusers.New(
		log,
		modelManager,
		log.With("component", diffusers.Name),
		nil,
		diffusersServerPath,
	)

	if err != nil {
		log.Error(fmt.Sprintf("unable to initialize diffusers backend: %v", err))
	}

	var vllmMetalBackend inference.Backend
	if platform.SupportsVLLMMetal() {
		vllmMetalBackend, err = vllmmetal.New(
			log,
			modelManager,
			log.With("component", vllmmetal.Name),
			vllmMetalServerPath,
		)
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to initialize vllm-metal backend: %v", err))
		}
	}

	backends := map[string]inference.Backend{
		llamacpp.Name:  llamaCppBackend,
		mlx.Name:       mlxBackend,
		sglang.Name:    sglangBackend,
		diffusers.Name: diffusersBackend,
	}
	registerVLLMBackend(backends, vllmBackend)

	if vllmMetalBackend != nil {
		backends[vllmmetal.Name] = vllmMetalBackend
	}

	// Backends whose installation is deferred until explicitly requested.
	var deferredBackends []string
	if vllmMetalBackend != nil {
		deferredBackends = append(deferredBackends, vllmmetal.Name)
	}

	scheduler := scheduling.NewScheduler(
		log,
		backends,
		llamaCppBackend,
		modelManager,
		http.DefaultClient,
		metrics.NewTracker(
			http.DefaultClient,
			log.With("component", "metrics"),
			"",
			false,
		),
		deferredBackends,
	)

	// Create the HTTP handler for the scheduler
	schedulerHTTP := scheduling.NewHTTPHandler(scheduler, modelHandler, nil)

	router := routing.NewNormalizedServeMux()

	// Register path prefixes to forward all HTTP methods (including OPTIONS) to components
	// Components handle method routing internally
	// Register both with and without trailing slash to avoid redirects
	router.Handle(inference.ModelsPrefix, modelHandler)
	router.Handle(inference.ModelsPrefix+"/", modelHandler)
	router.Handle(inference.InferencePrefix+"/", schedulerHTTP)
	// Add OpenAI Responses API compatibility layer
	responsesHandler := responses.NewHTTPHandler(log, schedulerHTTP, nil)
	router.Handle(responses.APIPrefix+"/", responsesHandler)
	router.Handle(responses.APIPrefix, responsesHandler) // Also register for exact match without trailing slash
	router.Handle("/v1"+responses.APIPrefix+"/", responsesHandler)
	router.Handle("/v1"+responses.APIPrefix, responsesHandler)
	// Also register Responses API under inference prefix to support all inference engines
	router.Handle(inference.InferencePrefix+responses.APIPrefix+"/", responsesHandler)
	router.Handle(inference.InferencePrefix+responses.APIPrefix, responsesHandler)

	// Add path aliases: /v1 -> /engines/v1, /rerank -> /engines/rerank, /score -> /engines/score.
	aliasHandler := &middleware.AliasHandler{Handler: schedulerHTTP}
	router.Handle("/v1/", aliasHandler)
	router.Handle("/rerank", aliasHandler)
	router.Handle("/score", aliasHandler)

	// Add Ollama API compatibility layer (only register with trailing slash to catch sub-paths)
	ollamaHandler := ollama.NewHTTPHandler(log, scheduler, schedulerHTTP, nil, modelManager)
	router.Handle(ollama.APIPrefix+"/", ollamaHandler)

	// Add Anthropic Messages API compatibility layer
	anthropicHandler := anthropic.NewHandler(log, schedulerHTTP, nil, modelManager)
	router.Handle(anthropic.APIPrefix+"/", anthropicHandler)

	// Register root handler LAST - it will only catch exact "/" requests that don't match other patterns
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Only respond to exact root path
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Docker Model Runner is running"))
	})

	// Add metrics endpoint if enabled
	if os.Getenv("DISABLE_METRICS") != "1" {
		metricsHandler := metrics.NewAggregatedMetricsHandler(
			log.With("component", "metrics"),
			schedulerHTTP,
		)
		router.Handle("/metrics", metricsHandler)
		log.Info("Metrics endpoint enabled at /metrics")
	} else {
		log.Info("Metrics endpoint disabled")
	}

	server := &http.Server{
		Handler:           router,
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
		log.Info(fmt.Sprintf("Listening on TCP port %s", tcpPort))
		server.Addr = addr
		go func() {
			serverErrors <- server.ListenAndServe()
		}()
	} else {
		// Use Unix socket
		if err := os.Remove(sockName); err != nil {
			if !os.IsNotExist(err) {
				log.Error(fmt.Sprintf("Failed to remove existing socket: %v", err))
			}
		}
		ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: sockName, Net: "unix"})
		if err != nil {
			log.Error(fmt.Sprintf("Failed to listen on socket: %v", err))
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
					log.Error(fmt.Sprintf("Failed to ensure TLS certificates: %v", err))
				}
				log.Info(fmt.Sprintf("Using TLS certificate: %s", certPath))
				log.Info(fmt.Sprintf("Using TLS key: %s", keyPath))
			} else {
				log.Error("TLS enabled but no certificate provided and auto-cert is disabled")
			}
		}

		// Load TLS configuration
		tlsConfig, err := modeltls.LoadTLSConfig(certPath, keyPath)
		if err != nil {
			log.Error(fmt.Sprintf("Failed to load TLS configuration: %v", err))
		}

		tlsServer = &http.Server{
			Addr:              ":" + tlsPort,
			Handler:           router,
			TLSConfig:         tlsConfig,
			ReadHeaderTimeout: 10 * time.Second,
		}

		log.Info(fmt.Sprintf("Listening on TLS port %s", tlsPort))
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
		schedulerErrors <- scheduler.Run(ctx)
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
			log.Error(fmt.Sprintf("Server error: %v", err))
		}
	case err := <-tlsServerErrorsChan:
		if err != nil {
			log.Error(fmt.Sprintf("TLS server error: %v", err))
		}
	case <-ctx.Done():
		log.Info("Shutdown signal received")
		log.Info("Shutting down the server")
		if err := server.Close(); err != nil {
			log.Error(fmt.Sprintf("Server shutdown error: %v", err))
		}
		if tlsServer != nil {
			log.Info("Shutting down the TLS server")
			if err := tlsServer.Close(); err != nil {
				log.Error(fmt.Sprintf("TLS server shutdown error: %v", err))
			}
		}
		log.Info("Waiting for the scheduler to stop")
		if err := <-schedulerErrors; err != nil {
			log.Error(fmt.Sprintf("Scheduler error: %v", err))
		}
	}
	log.Info("Docker Model Runner stopped")
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
				testLog.Error(fmt.Sprintf("LLAMA_ARGS cannot override the %s argument as it is controlled by the model runner", disallowed))
			}
		}
	}

	testLog.Info(fmt.Sprintf("Using custom arguments: %v", args))
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
