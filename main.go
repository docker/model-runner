package main

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/docker/model-runner/pkg/distribution/transport/resumable"
	"github.com/docker/model-runner/pkg/gpuinfo"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends/llamacpp"
	"github.com/docker/model-runner/pkg/inference/backends/vllm"
	"github.com/docker/model-runner/pkg/inference/config"
	"github.com/docker/model-runner/pkg/inference/memory"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/inference/scheduling"
	"github.com/docker/model-runner/pkg/metrics"
	"github.com/docker/model-runner/pkg/routing"
	"github.com/mattn/go-shellwords"
	"github.com/sirupsen/logrus"
)

var log = logrus.New()

// V1AliasHandler provides an alias from /v1/ to /engines/v1/ paths
type V1AliasHandler struct {
	scheduler http.Handler
}

func (h *V1AliasHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Modify the URL path to prepend /engines/ before /v1/
	originalPath := r.URL.Path
	newPath := inference.InferencePrefix + originalPath // originalPath is like "/v1/models", so result is "/engines/v1/models"

	// Create a clone of the request with the modified path
	r2 := new(http.Request)
	*r2 = *r
	r2.URL = new(url.URL)
	*r2.URL = *r.URL
	r2.URL.Path = newPath

	// Pass the modified request to the scheduler
	h.scheduler.ServeHTTP(w, r2)
}

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

	modelManager := models.NewManager(
		log,
		models.ClientConfig{
			StoreRootPath: modelPath,
			Logger:        log.WithFields(logrus.Fields{"component": "model-manager"}),
			Transport:     resumable.New(baseTransport),
		},
		nil,
		memEstimator,
	)

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

	scheduler := scheduling.NewScheduler(
		log,
		map[string]inference.Backend{llamacpp.Name: llamaCppBackend, vllm.Name: vllmBackend},
		llamaCppBackend,
		modelManager,
		http.DefaultClient,
		nil,
		metrics.NewTracker(
			http.DefaultClient,
			log.WithField("component", "metrics"),
			"",
			false,
		),
		sysMemInfo,
	)

	router := routing.NewNormalizedServeMux()

	// Register path prefixes to forward all HTTP methods (including OPTIONS) to components
	// Components handle method routing internally
	// Register both with and without trailing slash to avoid redirects
	router.Handle(inference.ModelsPrefix, modelManager)
	router.Handle(inference.ModelsPrefix+"/", modelManager)
	router.Handle(inference.InferencePrefix+"/", scheduler)
	// Add /v1 as an alias for /engines/v1
	router.Handle("/v1/", &V1AliasHandler{scheduler: scheduler})

	// Add metrics endpoint if enabled
	if os.Getenv("DISABLE_METRICS") != "1" {
		metricsHandler := metrics.NewAggregatedMetricsHandler(
			log.WithField("component", "metrics"),
			scheduler,
		)
		router.Handle("/metrics", metricsHandler)
		log.Info("Metrics endpoint enabled at /metrics")
	} else {
		log.Info("Metrics endpoint disabled")
	}

	server := &http.Server{Handler: router}
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
	args, err := shellwords.Parse(argsStr)
	if err != nil {
		log.Fatalf("Failed to parse LLAMA_ARGS: %v", err)
	}

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
