package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/docker/model-runner/pkg/anthropic"
	"github.com/docker/model-runner/pkg/envconfig"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends/llamacpp"
	"github.com/docker/model-runner/pkg/inference/backends/sglang"
	"github.com/docker/model-runner/pkg/inference/config"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/logging"
	dmrlogs "github.com/docker/model-runner/pkg/logs"
	"github.com/docker/model-runner/pkg/metrics"
	"github.com/docker/model-runner/pkg/ollama"
	"github.com/docker/model-runner/pkg/responses"
	"github.com/docker/model-runner/pkg/router"
	"github.com/docker/model-runner/pkg/routing"
	modeltls "github.com/docker/model-runner/pkg/tls"
)

// initLogger creates the application logger based on LOG_LEVEL env var.
func initLogger() *slog.Logger {
	return logging.NewLogger(envconfig.LogLevel())
}

var log = initLogger()

// exitFunc is used for Fatal-like exits; overridden in tests.
var exitFunc = func(code int) { os.Exit(code) }

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// sockName is the public-facing socket the Rust router listens on.
	sockName := envconfig.SocketPath()
	modelPath, err := envconfig.ModelsPath()
	if err != nil {
		log.Error("Failed to get models path", "error", err)
		exitFunc(1)
	}

	if envconfig.DisableServerUpdate() {
		llamacpp.ShouldUpdateServerLock.Lock()
		llamacpp.ShouldUpdateServer = false
		llamacpp.ShouldUpdateServerLock.Unlock()
	}

	if v := envconfig.LlamaServerVersion(); v != "" {
		llamacpp.SetDesiredServerVersion(v)
	}

	llamaServerPath := envconfig.LlamaServerPath()
	vllmServerPath := envconfig.VLLMServerPath()
	sglangServerPath := envconfig.SGLangServerPath()
	mlxServerPath := envconfig.MLXServerPath()
	diffusersServerPath := envconfig.DiffusersServerPath()
	vllmMetalServerPath := envconfig.VLLMMetalServerPath()

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
	if diffusersServerPath != "" {
		log.Info("DIFFUSERS_SERVER_PATH", "path", diffusersServerPath)
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
				IncludeDiffusers:     true,
				DiffusersPath:        diffusersServerPath,
			}),
			routing.BackendDef{Name: sglang.Name, Init: func(mm *models.Manager) (inference.Backend, error) {
				return sglang.New(log, mm, log.With("component", sglang.Name), nil, sglangServerPath)
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
		AllowedOrigins: envconfig.AllowedOrigins(),
	})
	if err != nil {
		log.Error("failed to initialize service", "error", err)
		exitFunc(1)
	}

	// Build the backend HTTP mux. Routing (path aliasing, CORS, path
	// normalisation, /version, /) is handled by the Rust dmr-router sidecar
	// that sits in front of this server. We only need to register the
	// inference endpoints and the observability endpoints that the Rust router
	// proxies through.
	mux := http.NewServeMux()
	mux.Handle(inference.InferencePrefix+"/", svc.SchedulerHTTP)
	mux.Handle(inference.ModelsPrefix+"/", svc.ModelHandler)
	mux.Handle(inference.ModelsPrefix, svc.ModelHandler)

	// Ollama API compatibility layer (/api/).
	ollamaHandler := ollama.NewHTTPHandler(log, svc.Scheduler, svc.SchedulerHTTP, envconfig.AllowedOrigins(), svc.ModelManager)
	mux.Handle(ollama.APIPrefix+"/", ollamaHandler)

	// Anthropic Messages API compatibility layer (/anthropic/).
	anthropicHandler := anthropic.NewHandler(log, svc.SchedulerHTTP, envconfig.AllowedOrigins(), svc.ModelManager)
	mux.Handle(anthropic.APIPrefix+"/", anthropicHandler)

	// OpenAI Responses API compatibility layer (/responses, /v1/responses, /engines/responses).
	responsesHandler := responses.NewHTTPHandler(log, svc.SchedulerHTTP, envconfig.AllowedOrigins())
	mux.Handle(responses.APIPrefix+"/", responsesHandler)
	mux.Handle(responses.APIPrefix, responsesHandler)
	mux.Handle("/v1"+responses.APIPrefix+"/", responsesHandler)
	mux.Handle("/v1"+responses.APIPrefix, responsesHandler)
	mux.Handle(inference.InferencePrefix+responses.APIPrefix+"/", responsesHandler)
	mux.Handle(inference.InferencePrefix+responses.APIPrefix, responsesHandler)

	// Logs endpoint (Docker Desktop mode only).
	if logDir := envconfig.LogDir(); logDir != "" {
		mux.HandleFunc("GET /logs", dmrlogs.NewHTTPHandler(logDir))
		log.Info("Logs endpoint enabled at /logs", "dir", logDir)
	}

	// Metrics endpoint.
	if !envconfig.DisableMetrics() {
		metricsHandler := metrics.NewAggregatedMetricsHandler(
			log.With("component", "metrics"),
			svc.SchedulerHTTP,
		)
		mux.Handle("/metrics", metricsHandler)
		log.Info("Metrics endpoint enabled at /metrics")
	} else {
		log.Info("Metrics endpoint disabled")
	}

	// ── Register the Go mux as the in-process Rust router backend ────────────
	// The Rust router calls Go's http.Handler directly via CGo for every
	// inference request — no second socket needed.  The streaming writer in
	// handler.go pushes chunks to Rust via dmr_write_chunk() as they are
	// written, so streaming endpoints like POST /models/create work correctly.
	handlerFn, handlerCtx := router.RegisterHandler(mux)

	// routerCfg is populated below depending on TCP vs Unix socket mode.
	routerCfg := router.Config{
		HandlerFn:      handlerFn,
		HandlerCtx:     handlerCtx,
		AllowedOrigins: envconfig.AllowedOrigins(),
		Version:        Version,
	}

	serverErrors := make(chan error, 1) // never fires in in-process mode

	// TLS server (optional) — serves the mux directly on a TCP port.
	var tlsServer *http.Server
	tlsServerErrors := make(chan error, 1)

	tcpPort := envconfig.TCPPort()
	if tcpPort != "" {
		backendPort, err := parsePort(tcpPort)
		if err != nil {
			log.Error("Invalid TCP_PORT", "error", err)
			exitFunc(1)
		}
		routerCfg.ListenPort = uint16(backendPort)
		log.Info("Rust router listening on TCP port", "port", backendPort)
	} else {
		routerCfg.ListenSock = sockName
		log.Info("Rust router listening on Unix socket", "path", sockName)
	}

	// ── TLS server (optional) ─────────────────────────────────────────────────
	if envconfig.TLSEnabled() {
		tlsPort := envconfig.TLSPort()
		certPath := envconfig.TLSCert()
		keyPath := envconfig.TLSKey()

		if certPath == "" || keyPath == "" {
			if envconfig.TLSAutoCert(true) {
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

		tlsConfig, err := modeltls.LoadTLSConfig(certPath, keyPath)
		if err != nil {
			log.Error("Failed to load TLS configuration", "error", err)
			exitFunc(1)
		}

		tlsServer = &http.Server{
			Addr:              ":" + tlsPort,
			Handler:           mux,
			TLSConfig:         tlsConfig,
			ReadHeaderTimeout: 10 * time.Second,
		}

		log.Info("Listening on TLS port", "port", tlsPort)
		go func() {
			ln, err := tls.Listen("tcp", tlsServer.Addr, tlsConfig)
			if err != nil {
				tlsServerErrors <- err
				return
			}
			tlsServerErrors <- tlsServer.Serve(ln)
		}()
	}

	// ── Rust router ───────────────────────────────────────────────────────────
	// router.Start launches the axum server in a background goroutine and
	// returns a StopFunc for graceful shutdown plus an error channel.
	stopRouter, routerErrors := router.Start(routerCfg)
	log.Info("Rust router started", "listen", routerCfg.ListenSock)

	// ── Scheduler ─────────────────────────────────────────────────────────────
	schedulerErrors := make(chan error, 1)
	go func() {
		schedulerErrors <- svc.Scheduler.Run(ctx)
	}()

	var tlsServerErrorsChan <-chan error
	if envconfig.TLSEnabled() {
		tlsServerErrorsChan = tlsServerErrors
	} else {
		tlsServerErrorsChan = nil
	}

	select {
	case err := <-routerErrors:
		if err != nil {
			log.Error("Rust router error", "error", err)
		}
	case err := <-tlsServerErrorsChan:
		if err != nil {
			log.Error("TLS server error", "error", err)
		}
	case err := <-serverErrors:
		if err != nil {
			log.Error("Backend server error", "error", err)
		}
	case <-ctx.Done():
		log.Info("Shutdown signal received")

		log.Info("Stopping Rust router")
		if stopRouter != nil {
			stopRouter()
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

// parsePort parses a decimal port string and returns an int.
func parsePort(s string) (int, error) {
	var p int
	if _, err := fmt.Sscanf(s, "%d", &p); err != nil {
		return 0, fmt.Errorf("invalid port %q: %w", s, err)
	}
	return p, nil
}

// createLlamaCppConfigFromEnv creates a LlamaCppConfig from environment variables.
// Returns nil config (use defaults) when LLAMA_ARGS is unset, or an error if
// the args contain disallowed flags.
func createLlamaCppConfigFromEnv() (config.BackendConfig, error) {
	argsStr := envconfig.LlamaArgs()
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
