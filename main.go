package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/docker/model-runner/pkg/anthropic"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends/diffusers"
	"github.com/docker/model-runner/pkg/inference/backends/llamacpp"
	"github.com/docker/model-runner/pkg/inference/backends/mlx"
	"github.com/docker/model-runner/pkg/inference/backends/sglang"
	"github.com/docker/model-runner/pkg/inference/backends/vllm"
	"github.com/docker/model-runner/pkg/inference/config"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/inference/scheduling"
	"github.com/docker/model-runner/pkg/metrics"
	"github.com/docker/model-runner/pkg/middleware"
	"github.com/docker/model-runner/pkg/ollama"
	"github.com/docker/model-runner/pkg/responses"
	"github.com/docker/model-runner/pkg/routing"
	"github.com/mattn/go-shellwords"
	"github.com/sirupsen/logrus"
)

const (
	defaultSocketName      = "model-runner.sock"
	defaultModelsPath      = ".docker/models"
	defaultLlamaServerPath = "/Applications/Docker.app/Contents/Resources/model-runner/bin"
	socketFileMode         = 0o600
	defaultDirectoryMode   = 0o755
)

var log = logrus.New()

func initializeBackends(log *logrus.Logger, modelManager *models.Manager, llamaServerPath string, llamaCppConfig config.BackendConfig) (map[string]inference.Backend, inference.Backend, error) {
	// Initialize llama.cpp backend
	llamaCppBackend, err := initLlamaCppBackend(log, modelManager, llamaServerPath, llamaCppConfig)
	if err != nil {
		return nil, nil, err
	}

	// Initialize VLLM backend
	vllmBackend, err := initVLLMBackend(log, modelManager)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to initialize %s backend: %w", vllm.Name, err)
	}

	// Initialize other backends with explicit error handling
	mlxBackend, err := initMlxBackend(log, modelManager)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to initialize %s backend: %w", mlx.Name, err)
	}

	sglangBackend, err := initSglangBackend(log, modelManager)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to initialize %s backend: %w", sglang.Name, err)
	}

	diffusersBackend, err := initDiffusersBackend(log, modelManager)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to initialize %s backend: %w", diffusers.Name, err)
	}

	backends := map[string]inference.Backend{
		llamacpp.Name:  llamaCppBackend,
		mlx.Name:       mlxBackend,
		sglang.Name:    sglangBackend,
		diffusers.Name: diffusersBackend,
	}

	// Only register VLLM backend if it was properly initialized (not nil)
	if vllmBackend != nil {
		registerVLLMBackend(backends, vllmBackend)
	}

	return backends, llamaCppBackend, nil
}

func initLlamaCppBackend(log *logrus.Logger, modelManager *models.Manager, llamaServerPath string, llamaCppConfig config.BackendConfig) (inference.Backend, error) {
	backend, err := llamacpp.New(
		log,
		modelManager,
		log.WithFields(logrus.Fields{"component": llamacpp.Name}),
		llamaServerPath,
		getLlamaCppUpdateDir(log),
		llamaCppConfig,
	)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize %s backend: %w", llamacpp.Name, err)
	}
	return backend, nil
}

func initMlxBackend(log *logrus.Logger, modelManager *models.Manager) (inference.Backend, error) {
	backend, err := mlx.New(
		log,
		modelManager,
		log.WithFields(logrus.Fields{"component": mlx.Name}),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize %s backend: %w", mlx.Name, err)
	}
	return backend, nil
}

func initSglangBackend(log *logrus.Logger, modelManager *models.Manager) (inference.Backend, error) {
	backend, err := sglang.New(
		log,
		modelManager,
		log.WithFields(logrus.Fields{"component": sglang.Name}),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize %s backend: %w", sglang.Name, err)
	}
	return backend, nil
}

func initDiffusersBackend(log *logrus.Logger, modelManager *models.Manager) (inference.Backend, error) {
	backend, err := diffusers.New(
		log,
		modelManager,
		log.WithFields(logrus.Fields{"component": diffusers.Name}),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize %s backend: %w", diffusers.Name, err)
	}
	return backend, nil
}

func getLlamaCppUpdateDir(log *logrus.Logger) string {
	wd, err := os.Getwd()
	if err != nil {
		log.Errorf("Failed to get working directory, using current directory: %v", err)
		wd = "."
	}
	d := filepath.Join(wd, "updated-inference", "bin")
	if err := os.MkdirAll(d, defaultDirectoryMode); err != nil {
		log.Errorf("Failed to create directory %s: %v", d, err)
	}
	return d
}

func getSocketName() string {
	sockName := os.Getenv("MODEL_RUNNER_SOCK")
	if sockName == "" {
		sockName = defaultSocketName
	}
	return sockName
}

func getModelPath() string {
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to get user home directory: %v", err)
	}

	modelPath := os.Getenv("MODELS_PATH")
	if modelPath == "" {
		modelPath = filepath.Join(userHomeDir, defaultModelsPath)
	}
	return modelPath
}

func getLlamaServerPath() string {
	llamaServerPath := os.Getenv("LLAMA_SERVER_PATH")
	if llamaServerPath == "" {
		llamaServerPath = defaultLlamaServerPath
	}
	return llamaServerPath
}

func configureLlamaCpp() error {
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
	return nil
}

func createProxyTransport() *http.Transport {
	// Create a proxy-aware HTTP transport
	// Use a safe type assertion with fallback
	var baseTransport *http.Transport
	if t, ok := http.DefaultTransport.(*http.Transport); ok {
		baseTransport = t.Clone()
	} else {
		// Fallback to a default transport if type assertion fails
		baseTransport = &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		}
	}
	return baseTransport
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Initialize configuration and services
	config, err := initializeAppConfig()
	if err != nil {
		log.Fatalf("Failed to initialize app config: %v", err)
	}

	// Initialize backends
	backends, llamaCppBackend, err := initializeBackends(log, config.modelManager, config.llamaServerPath, config.llamaCppConfig)
	if err != nil {
		log.Fatalf("Failed to initialize backends: %v", err)
	}

	// Create scheduler
	scheduler := createScheduler(config, backends, llamaCppBackend)

	// Setup HTTP handlers
	router := setupHTTPHandlers(config, scheduler)

	// Start server
	server, serverErrors := startServer(router, config.sockName)

	// Start scheduler
	schedulerErrors := make(chan error, 1)
	go func() {
		schedulerErrors <- scheduler.Run(ctx)
	}()

	// Wait for shutdown
	waitForShutdown(ctx, server, serverErrors, schedulerErrors)
	log.Infoln("Docker Model Runner stopped")
}

// AppConfig holds the application configuration
type AppConfig struct {
	sockName        string
	modelPath       string
	llamaServerPath string
	modelManager    *models.Manager
	llamaCppConfig  config.BackendConfig
}

// initializeAppConfig initializes the application configuration
func initializeAppConfig() (*AppConfig, error) {
	sockName := getSocketName()
	modelPath := getModelPath()
	llamaServerPath := getLlamaServerPath()

	if err := configureLlamaCpp(); err != nil {
		return nil, fmt.Errorf("failed to configure llama.cpp: %w", err)
	}

	baseTransport := createProxyTransport()
	clientConfig := models.ClientConfig{
		StoreRootPath: modelPath,
		Logger:        log.WithFields(logrus.Fields{"component": "model-manager"}),
		Transport:     baseTransport,
	}
	modelManager := models.NewManager(log.WithFields(logrus.Fields{"component": "model-manager"}), clientConfig)
	log.Infof("LLAMA_SERVER_PATH: %s", llamaServerPath)

	// Create llama.cpp configuration from environment variables
	llamaCppConfig := createLlamaCppConfigFromEnv()

	return &AppConfig{
		sockName:        sockName,
		modelPath:       modelPath,
		llamaServerPath: llamaServerPath,
		modelManager:    modelManager,
		llamaCppConfig:  llamaCppConfig,
	}, nil
}

// createScheduler creates a new scheduler instance
func createScheduler(config *AppConfig, backends map[string]inference.Backend, llamaCppBackend inference.Backend) *scheduling.Scheduler {
	return scheduling.NewScheduler(
		log,
		backends,
		llamaCppBackend,
		config.modelManager,
		http.DefaultClient,
		metrics.NewTracker(
			http.DefaultClient,
			log.WithField("component", "metrics"),
			"",
			false,
		),
	)
}

// setupHTTPHandlers sets up all HTTP handlers for the application
func setupHTTPHandlers(config *AppConfig, scheduler *scheduling.Scheduler) *routing.NormalizedServeMux {
	modelHandler := models.NewHTTPHandler(
		log,
		config.modelManager,
		nil,
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
	ollamaHandler := ollama.NewHTTPHandler(log, scheduler, schedulerHTTP, nil, config.modelManager)
	router.Handle(ollama.APIPrefix+"/", ollamaHandler)

	// Add Anthropic Messages API compatibility layer
	anthropicHandler := anthropic.NewHandler(log, schedulerHTTP, nil, config.modelManager)
	router.Handle(anthropic.APIPrefix+"/", anthropicHandler)

	// Register root handler LAST - it will only catch exact "/" requests that don't match other patterns
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Only respond to exact root path
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Docker Model Runner is running"))
	})

	// Add metrics endpoint if enabled
	if os.Getenv("DISABLE_METRICS") != "1" {
		metricsHandler := metrics.NewAggregatedMetricsHandler(
			log.WithField("component", "metrics"),
			schedulerHTTP,
		)
		router.Handle("/metrics", metricsHandler)
		log.Info("Metrics endpoint enabled at /metrics")
	} else {
		log.Info("Metrics endpoint disabled")
	}

	return router
}

// startServer starts the HTTP server and returns the server instance and error channel
func startServer(router *routing.NormalizedServeMux, sockName string) (*http.Server, chan error) {
	server := &http.Server{
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}
	serverErrors := make(chan error, 1)

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
		// Set appropriate permissions on the socket file to restrict access
		if err := os.Chmod(sockName, socketFileMode); err != nil {
			log.Errorf("Failed to set socket file permissions: %v", err)
		}
		go func() {
			serverErrors <- server.Serve(ln)
		}()
	}

	return server, serverErrors
}

// waitForShutdown waits for shutdown signals and handles cleanup
func waitForShutdown(ctx context.Context, server *http.Server, serverErrors chan error, schedulerErrors chan error) {
	select {
	case err := <-serverErrors:
		if err != nil {
			log.Errorf("Server error: %v", err)
		}
	case <-ctx.Done():
		log.Infoln("Shutdown signal received")
		log.Infoln("Shutting down the server")
		if err := server.Close(); err != nil {
			log.Errorf("Server shutdown error: %v", err)
		}
		log.Infoln("Waiting for the scheduler to stop")
		if err := <-schedulerErrors; err != nil {
			log.Errorf("Scheduler error: %v", err)
		}
	}
}

// createLlamaCppConfigFromEnv creates a LlamaCppConfig from environment variables
func createLlamaCppConfigFromEnv() config.BackendConfig {
	// Check if any configuration environment variables are set
	argsStr := os.Getenv("LLAMA_ARGS")

	// If no environment variables are set, use default configuration
	if argsStr == "" {
		return nil // nil will cause the backend to use its default configuration
	}

	// Split the string by spaces, respecting quoted arguments using shellwords
	args, err := shellwords.Parse(argsStr)
	if err != nil {
		log.Errorf("Failed to parse LLAMA_ARGS: %v. Using default configuration.", err)
		return nil
	}

	// Check for disallowed arguments
	disallowedArgs := []string{"--model", "--host", "--embeddings", "--mmproj"}
	for _, arg := range args {
		for _, disallowed := range disallowedArgs {
			if isDisallowedArg(arg, disallowed) {
				log.Errorf("LLAMA_ARGS cannot override the %s argument as it is controlled by the model runner. Using default configuration.", disallowed)
				return nil
			}
		}
	}

	log.Infof("Using custom arguments: %v", args)
	return &llamacpp.Config{
		Args: args,
	}
}

// isDisallowedArg checks if an argument matches a disallowed argument pattern
func isDisallowedArg(arg, disallowed string) bool {
	return arg == disallowed || (strings.HasPrefix(arg, disallowed) && len(arg) > len(disallowed) && arg[len(disallowed)] == '=')
}
