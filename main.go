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

	modelManager := models.NewManager(log, models.ClientConfig{
		StoreRootPath: filepath.Join(userHomeDir, ".docker", "models"),
		Logger:        log.WithFields(logrus.Fields{"component": "model-manager"}),
	})

	llamaCppBackend, err := llamacpp.New(
		log,
		modelManager,
		log.WithFields(logrus.Fields{"component": "llama.cpp"}),
		"/Applications/Docker.app/Contents/Resources/bin",
		func() string { wd, _ := os.Getwd(); return wd }(),
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

	router := http.NewServeMux()
	for _, route := range modelManager.GetRoutes() {
		router.Handle(route, modelManager)
	}
	for _, route := range scheduler.GetRoutes() {
		router.Handle(route, scheduler)
	}

	if err := os.Remove(sockName); err != nil {
		if !os.IsNotExist(err) {
			log.Fatalf("Failed to remove existing socket: %v", err)
		}
	}
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: sockName, Net: "unix"})
	if err != nil {
		log.Fatalf("Failed to listen on socket: %v", err)
	}

	server := &http.Server{Handler: router}
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.Serve(ln)
	}()
	defer server.Close()

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
