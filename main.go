package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends/llamacpp"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/inference/scheduling"
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

	modelManager := models.NewManager(log, models.ClientConfig{
		StoreRootPath: modelPath,
		Logger:        log.WithFields(logrus.Fields{"component": "model-manager"}),
	})

	llamaServerPath := os.Getenv("LLAMA_SERVER_PATH")
	if llamaServerPath == "" {
		llamaServerPath = "/Applications/Docker.app/Contents/Resources/model-runner/bin"
	}

	log.Infof("LLAMA_SERVER_PATH: %s", llamaServerPath)

	llamaCppBackend, err := llamacpp.New(
		log,
		modelManager,
		log.WithFields(logrus.Fields{"component": "llama.cpp"}),
		llamaServerPath,
		func() string {
			wd, _ := os.Getwd()
			d := filepath.Join(wd, "updated-inference")
			_ = os.MkdirAll(d, 0o755)
			return d
		}(),
	)
	if err != nil {
		log.Fatalf("unable to initialize %s backend: %v", llamacpp.Name, err)
	}

	scheduler := scheduling.NewScheduler(
		log,
		map[string]inference.Backend{llamacpp.Name: llamaCppBackend},
		llamaCppBackend,
		modelManager,
		http.DefaultClient,
	)

	router := routing.NewNormalizedServeMux()
	for _, route := range modelManager.GetRoutes() {
		router.Handle(route, modelManager)
	}
	for _, route := range scheduler.GetRoutes() {
		router.Handle(route, scheduler)
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
		log.Infoln("Waiting for the scheduler to stop")
		if err := <-schedulerErrors; err != nil {
			log.Errorf("Scheduler error: %v", err)
		}
		log.Infoln("Shutting down the server")
		if err := server.Shutdown(ctx); err != nil {
			log.Errorf("Server shutdown error: %v", err)
		}
	}
	log.Infoln("Docker Model Runner stopped")
}
