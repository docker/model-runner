// Package server runs the model-runner HTTP daemon.
package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/model-runner/pkg/envconfig"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends/llamacpp"
	"github.com/docker/model-runner/pkg/inference/backends/ovms"
	"github.com/docker/model-runner/pkg/inference/backends/sglang"
	"github.com/docker/model-runner/pkg/inference/config"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/logging"
	dmrlogs "github.com/docker/model-runner/pkg/logs"
	"github.com/docker/model-runner/pkg/metrics"
	"github.com/docker/model-runner/pkg/routing"
	modeltls "github.com/docker/model-runner/pkg/tls"
)

// Config holds server startup options.
type Config struct {
	Version string
	// ExitFunc is called on fatal errors that cannot be propagated via the
	// callback signature (e.g. OnBackendError). Defaults to os.Exit.
	ExitFunc func(int)
}

// Run starts the HTTP server and blocks until ctx is cancelled.
func Run(ctx context.Context, cfg Config) error {
	exitFunc := cfg.ExitFunc
	if exitFunc == nil {
		exitFunc = os.Exit
	}
	log := logging.NewLogger(envconfig.LogLevel())

	sockName := envconfig.SocketPath()
	modelPath, err := envconfig.ModelsPath()
	if err != nil {
		return fmt.Errorf("failed to get models path: %w", err)
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
	ovmsServerPath := envconfig.OVMSServerPath()
	vllmMetalServerPath := envconfig.VLLMMetalServerPath()

	// Create a proxy-aware HTTP transport.
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
	if ovmsServerPath != "" {
		log.Info("OVMS_SERVER_PATH", "path", ovmsServerPath)
	}

	// Determine log directory.  When MODEL_RUNNER_LOG_DIR is set use it;
	// otherwise auto-create a default directory so that the /logs endpoint is
	// available in all deployment modes.
	logDir := envconfig.LogDir()
	if logDir == "" {
		logDir = filepath.Join(os.TempDir(), "model-runner-logs")
		if mkdirErr := os.MkdirAll(logDir, 0o755); mkdirErr != nil {
			log.Warn("failed to create default log directory, /logs endpoint will be disabled",
				"dir", logDir, "error", mkdirErr)
			logDir = ""
		}
	}

	// When a log directory is available, set up file-backed logging using tee
	// writers (stderr + bracket-timestamped files) so that both `docker logs`
	// and the /logs API work.
	var engineLogFile *os.File
	if logDir != "" {
		serviceLogFile, openErr := os.OpenFile(
			filepath.Join(logDir, dmrlogs.ServiceLogName),
			os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644,
		)
		if openErr != nil {
			log.Warn("failed to open service log file, /logs endpoint will be disabled", "error", openErr)
			logDir = ""
		} else {
			defer serviceLogFile.Close()
			bracketW := logging.NewBracketWriter(serviceLogFile)
			log = slog.New(slog.NewTextHandler(
				io.MultiWriter(os.Stderr, bracketW),
				&slog.HandlerOptions{Level: envconfig.LogLevel()},
			))
		}

		if logDir != "" {
			var openErr error
			engineLogFile, openErr = os.OpenFile(
				filepath.Join(logDir, dmrlogs.EngineLogName),
				os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644,
			)
			if openErr != nil {
				log.Warn("failed to open engine log file", "error", openErr)
			} else {
				defer engineLogFile.Close()
			}
		}
	}

	llamaCppConfig, err := createLlamaCppConfigFromEnv()
	if err != nil {
		return fmt.Errorf("invalid LLAMA_ARGS: %w", err)
	}

	svc, err := routing.NewService(routing.ServiceConfig{
		Log: log,
		ClientConfig: models.ClientConfig{
			StoreRootPath:   modelPath,
			Logger:          log.With("component", "model-manager"),
			Transport:       baseTransport,
			RegistryMirrors: envconfig.RegistryMirrors(),
		},
		Backends: append(
			routing.DefaultBackendDefs(routing.BackendsConfig{
				Log: log,
				ServerLogFactory: func(_ string) logging.Logger {
					if engineLogFile == nil {
						return log
					}
					bracketW := logging.NewBracketWriter(engineLogFile)
					return slog.New(slog.NewTextHandler(
						io.MultiWriter(os.Stderr, bracketW),
						&slog.HandlerOptions{Level: envconfig.LogLevel()},
					))
				},
				LlamaCppPath:     llamaServerPath,
				LlamaCppConfig:   llamaCppConfig,
				IncludeMLX:       true,
				MLXPath:          mlxServerPath,
				IncludeVLLM:      includeVLLM,
				VLLMPath:         vllmServerPath,
				VLLMMetalPath:    vllmMetalServerPath,
				IncludeDiffusers: true,
				DiffusersPath:    diffusersServerPath,
				RegistryMirrors:  envconfig.RegistryMirrors(),
			}),
			routing.BackendDef{Name: ovms.Name, Init: func(mm *models.Manager) (inference.Backend, error) {
				return ovms.New(log, mm, log.With("component", ovms.Name), ovmsServerPath)
			}},
			routing.BackendDef{Name: sglang.Name, Init: func(mm *models.Manager) (inference.Backend, error) {
				return sglang.New(log, mm, log.With("component", sglang.Name), nil, sglangServerPath, nil)
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
		AllowedOrigins:      envconfig.AllowedOrigins(),
		IncludeResponsesAPI: true,
		ExtraRoutes: func(r *routing.NormalizedServeMux, s *routing.Service) {
			// Root handler – only catches exact "/" requests.
			r.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
				if req.URL.Path != "/" {
					http.NotFound(w, req)
					return
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("Docker Model Runner is running"))
			})

			// Version endpoint.
			r.HandleFunc("/version", func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if encErr := json.NewEncoder(w).Encode(map[string]string{"version": cfg.Version}); encErr != nil {
					log.Warn("failed to write version response", "error", encErr)
				}
			})

			// Logs endpoint – available when a log directory exists.
			if logDir != "" {
				r.HandleFunc(
					"GET /logs",
					dmrlogs.NewHTTPHandler(logDir),
				)
				log.Info("Logs endpoint enabled at /logs", "dir", logDir)
			}

			// Metrics endpoint.
			if !envconfig.DisableMetrics() {
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
		return fmt.Errorf("failed to initialize service: %w", err)
	}
	defer svc.Close()

	httpServer := &http.Server{
		Handler:           svc.Router,
		ReadHeaderTimeout: 10 * time.Second,
	}
	serverErrors := make(chan error, 1)

	// TLS server (optional).
	var tlsServer *http.Server
	tlsServerErrors := make(chan error, 1)

	// Use TCP port when MODEL_RUNNER_PORT is set; otherwise Unix socket.
	tcpPort := envconfig.TCPPort()
	if tcpPort != "" {
		addr := ":" + tcpPort
		log.Info("Listening on TCP port", "port", tcpPort)
		httpServer.Addr = addr
		go func() {
			serverErrors <- httpServer.ListenAndServe()
		}()
	} else {
		if err := os.Remove(sockName); err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("failed to remove existing socket: %w", err)
			}
		}
		ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: sockName, Net: "unix"})
		if err != nil {
			return fmt.Errorf("failed to listen on socket: %w", err)
		}
		go func() {
			serverErrors <- httpServer.Serve(ln)
		}()
	}

	// Start TLS server if enabled.
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
					return fmt.Errorf("failed to ensure TLS certificates: %w", err)
				}
				log.Info("Using TLS certificate", "cert", certPath)
				log.Info("Using TLS key", "key", keyPath)
			} else {
				return fmt.Errorf("TLS enabled but no certificate provided and auto-cert is disabled")
			}
		}

		tlsConfig, err := modeltls.LoadTLSConfig(certPath, keyPath)
		if err != nil {
			return fmt.Errorf("failed to load TLS configuration: %w", err)
		}

		tlsServer = &http.Server{
			Addr:              ":" + tlsPort,
			Handler:           svc.Router,
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

	schedulerErrors := make(chan error, 1)
	go func() {
		schedulerErrors <- svc.Scheduler.Run(ctx)
	}()

	var tlsServerErrorsChan <-chan error
	if envconfig.TLSEnabled() {
		tlsServerErrorsChan = tlsServerErrors
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
		if err := httpServer.Close(); err != nil {
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
	return nil
}

// createLlamaCppConfigFromEnv builds a LlamaCppConfig from the LLAMA_ARGS
// environment variable.  Returns nil config (use defaults) when LLAMA_ARGS is
// unset, or an error if the args contain disallowed flags.
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

	return &llamacpp.Config{Args: args}, nil
}

// splitArgs splits s into arguments, respecting quoted strings.
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
