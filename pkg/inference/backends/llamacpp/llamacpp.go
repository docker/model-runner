package llamacpp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/logging"
)

const (
	// Name is the backend name.
	Name = "llama.cpp"
)

// llamaCpp is the llama.cpp-based backend implementation.
type llamaCpp struct {
	// log is the associated logger.
	log logging.Logger
	// modelManager is the shared model manager.
	modelManager *models.Manager
	// serverLog is the logger to use for the llama.cpp server process.
	serverLog       logging.Logger
	updatedLlamaCpp bool
	// vendoredServerStoragePath is the parent path of the vendored version of com.docker.llama-server.
	vendoredServerStoragePath string
	// updatedServerStoragePath is the parent path of the updated version of com.docker.llama-server.
	// It is also where updates will be stored when downloaded.
	updatedServerStoragePath string
}

// New creates a new llama.cpp-based backend.
func New(
	log logging.Logger,
	modelManager *models.Manager,
	serverLog logging.Logger,
	vendoredServerStoragePath string,
	updatedServerStoragePath string,
) (inference.Backend, error) {
	return &llamaCpp{
		log:                       log,
		modelManager:              modelManager,
		serverLog:                 serverLog,
		vendoredServerStoragePath: vendoredServerStoragePath,
		updatedServerStoragePath:  updatedServerStoragePath,
	}, nil
}

// Name implements inference.Backend.Name.
func (l *llamaCpp) Name() string {
	return Name
}

// UsesExternalModelManagement implements
// inference.Backend.UsesExternalModelManagement.
func (l *llamaCpp) UsesExternalModelManagement() bool {
	return false
}

// Install implements inference.Backend.Install.
func (l *llamaCpp) Install(ctx context.Context, httpClient *http.Client) error {
	// We don't currently support this backend on Windows or Linux. We'll likely
	// never support it on Intel Macs.
	if runtime.GOOS == "windows" || runtime.GOOS == "linux" {
		return errors.New("not implemented")
	} else if runtime.GOOS == "darwin" && runtime.GOARCH == "amd64" {
		return errors.New("platform not supported")
	}

	// Temporary workaround for dynamically downloading llama.cpp from Docker Hub.
	// Internet access and an available docker/docker-model-backend-llamacpp:latest-update on Docker Hub are required.
	// Even if docker/docker-model-backend-llamacpp:latest-update has been downloaded before, we still require its
	// digest to be equal to the one on Docker Hub.
	llamaCppPath := filepath.Join(l.updatedServerStoragePath, "com.docker.llama-server")
	if err := ensureLatestLlamaCpp(ctx, l.log, httpClient, llamaCppPath); err != nil {
		l.log.Infof("failed to ensure latest llama.cpp: %v\n", err)
		if errors.Is(err, context.Canceled) {
			return err
		}
	} else {
		l.updatedLlamaCpp = true
	}

	return nil
}

// Run implements inference.Backend.Run.
func (l *llamaCpp) Run(ctx context.Context, socket, model string, mode inference.BackendMode) error {
	modelPath, err := l.modelManager.GetModelPath(model)
	l.log.Infof("Model path: %s", modelPath)
	if err != nil {
		return fmt.Errorf("failed to get model path: %w", err)
	}

	if err := os.RemoveAll(socket); err != nil {
		l.log.Warnln("failed to remove socket file %s: %w", socket, err)
		l.log.Warnln("llama.cpp may not be able to start")
	}

	binPath := l.vendoredServerStoragePath
	if l.updatedLlamaCpp {
		binPath = l.updatedServerStoragePath
	}
	llamaCppArgs := []string{"--model", modelPath, "--jinja"}
	if mode == inference.BackendModeEmbedding {
		llamaCppArgs = append(llamaCppArgs, "--embeddings")
	}
	llamaCppProcess := exec.CommandContext(
		ctx,
		filepath.Join(binPath, "com.docker.llama-server"),
		llamaCppArgs...,
	)
	llamaCppProcess.Env = append(os.Environ(),
		"DD_INF_UDS="+socket,
	)
	llamaCppProcess.Cancel = func() error {
		// TODO: Figure out the correct process to send on Windows if/when we
		// port this backend there.
		return llamaCppProcess.Process.Signal(os.Interrupt)
	}
	serverLogStream := l.serverLog.Writer()
	llamaCppProcess.Stdout = serverLogStream
	llamaCppProcess.Stderr = serverLogStream

	if err := llamaCppProcess.Start(); err != nil {
		return fmt.Errorf("unable to start llama.cpp: %w", err)
	}

	llamaCppErrors := make(chan error, 1)
	go func() {
		llamaCppErr := llamaCppProcess.Wait()
		serverLogStream.Close()
		llamaCppErrors <- llamaCppErr
		close(llamaCppErrors)
	}()
	defer func() {
		<-llamaCppErrors
	}()

	select {
	case <-ctx.Done():
		return nil
	case llamaCppErr := <-llamaCppErrors:
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		return fmt.Errorf("llama.cpp terminated unexpectedly: %w", llamaCppErr)
	}
}
