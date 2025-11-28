package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/docker/model-runner/pkg/gpuinfo"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends/llamacpp"
	"github.com/docker/model-runner/pkg/inference/backends/mlx"
	"github.com/docker/model-runner/pkg/inference/backends/vllm"
	"github.com/docker/model-runner/pkg/inference/config"
	"github.com/docker/model-runner/pkg/inference/memory"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/inference/scheduling"
	"github.com/docker/model-runner/pkg/metrics"
	"github.com/docker/model-runner/pkg/middleware"
	"github.com/docker/model-runner/pkg/ollama"
	"github.com/docker/model-runner/pkg/routing"
	"github.com/sirupsen/logrus"
)

var log = logrus.New()

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

	gpuInfo := gpuinfo.New(llamaServerPath)

	sysMemInfo, err := memory.NewSystemMemoryInfo(log, gpuInfo)
	if err != nil {
		log.Fatalf("unable to initialize system memory info: %v", err)
	}

	memEstimator := memory.NewEstimator(sysMemInfo)

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
		Logger:        log.WithFields(logrus.Fields{"component": "model-manager"}),
		Transport:     baseTransport,
	}
	modelHandler := models.NewHTTPHandler(
		log,
		clientConfig,
		nil,
		memEstimator,
	)
	modelManager := models.NewManager(log.WithFields(logrus.Fields{"component": "model-manager"}), clientConfig)
	log.Infof("LLAMA_SERVER_PATH: %s", llamaServerPath)

	// Create llama.cpp configuration from environment variables
	llamaCppConfig := createLlamaCppConfigFromEnv()

	llamaCppBackend, err := llamacpp.New(
		log,
		modelManager,
		log.WithFields(logrus.Fields{"component": llamacpp.Name}),
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
		log.Fatalf("unable to initialize %s backend: %v", llamacpp.Name, err)
	}

	if os.Getenv("MODEL_RUNNER_RUNTIME_MEMORY_CHECK") == "1" {
		memory.SetRuntimeMemoryCheck(true)
	}

	memEstimator.SetDefaultBackend(llamaCppBackend)

	vllmBackend, err := vllm.New(
		log,
		modelManager,
		log.WithFields(logrus.Fields{"component": vllm.Name}),
		nil,
	)
	if err != nil {
		log.Fatalf("unable to initialize %s backend: %v", vllm.Name, err)
	}

	mlxBackend, err := mlx.New(
		log,
		modelManager,
		log.WithFields(logrus.Fields{"component": mlx.Name}),
		nil,
	)
	if err != nil {
		log.Fatalf("unable to initialize %s backend: %v", mlx.Name, err)
	}

	scheduler := scheduling.NewScheduler(
		log,
		map[string]inference.Backend{
			llamacpp.Name: llamaCppBackend,
			vllm.Name:     vllmBackend,
			mlx.Name:      mlxBackend,
		},
		llamaCppBackend,
		modelManager,
		http.DefaultClient,
		metrics.NewTracker(
			http.DefaultClient,
			log.WithField("component", "metrics"),
			"",
			false,
		),
		sysMemInfo,
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
	// Add path aliases: /v1 -> /engines/v1, /rerank -> /engines/rerank, /score -> /engines/score.
	aliasHandler := &middleware.AliasHandler{Handler: schedulerHTTP}
	router.Handle("/v1/", aliasHandler)
	router.Handle("/rerank", aliasHandler)
	router.Handle("/score", aliasHandler)

	// Add Ollama API compatibility layer (only register with trailing slash to catch sub-paths)
	ollamaHandler := ollama.NewHTTPHandler(log, scheduler, schedulerHTTP, nil, modelManager)
	router.Handle(ollama.APIPrefix+"/", ollamaHandler)

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
		go func() {
			serverErrors <- server.Serve(ln)
		}()
	}

	schedulerErrors := make(chan error, 1)
	go func() {
		schedulerErrors <- scheduler.Run(ctx)
	}()

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
				log.Fatalf("LLAMA_ARGS cannot override the %s argument as it is controlled by the model runner", disallowed)
			}
		}
	}

	log.Infof("Using custom arguments: %v", args)
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
