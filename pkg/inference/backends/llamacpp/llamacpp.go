package llamacpp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	parser "github.com/gpustack/gguf-parser-go"

	"github.com/docker/model-runner/pkg/diskusage"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/config"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/logging"
	"github.com/docker/model-runner/pkg/tailbuffer"
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
	// status is the state in which the llama.cpp backend is in.
	status string
	// config is the configuration for the llama.cpp backend.
	config config.BackendConfig
}

// New creates a new llama.cpp-based backend.
func New(
	log logging.Logger,
	modelManager *models.Manager,
	serverLog logging.Logger,
	vendoredServerStoragePath string,
	updatedServerStoragePath string,
	conf config.BackendConfig,
) (inference.Backend, error) {
	// If no config is provided, use the default configuration
	if conf == nil {
		conf = NewDefaultLlamaCppConfig()
	}

	return &llamaCpp{
		log:                       log,
		modelManager:              modelManager,
		serverLog:                 serverLog,
		vendoredServerStoragePath: vendoredServerStoragePath,
		updatedServerStoragePath:  updatedServerStoragePath,
		config:                    conf,
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
	l.updatedLlamaCpp = false

	// We don't currently support this backend on Windows. We'll likely
	// never support it on Intel Macs.
	if (runtime.GOOS == "darwin" && runtime.GOARCH == "amd64") ||
		(runtime.GOOS == "windows" && !(runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64")) {
		return errors.New("platform not supported")
	}

	llamaServerBin := "com.docker.llama-server"
	if runtime.GOOS == "windows" {
		llamaServerBin = "com.docker.llama-server.exe"
	}

	l.status = "installing"

	// Temporary workaround for dynamically downloading llama.cpp from Docker Hub.
	// Internet access and an available docker/docker-model-backend-llamacpp:latest on Docker Hub are required.
	// Even if docker/docker-model-backend-llamacpp:latest has been downloaded before, we still require its
	// digest to be equal to the one on Docker Hub.
	llamaCppPath := filepath.Join(l.updatedServerStoragePath, llamaServerBin)
	if err := l.ensureLatestLlamaCpp(ctx, l.log, httpClient, llamaCppPath, l.vendoredServerStoragePath); err != nil {
		l.log.Infof("failed to ensure latest llama.cpp: %v\n", err)
		if !(errors.Is(err, errLlamaCppUpToDate) || errors.Is(err, errLlamaCppUpdateDisabled)) {
			l.status = fmt.Sprintf("failed to install llama.cpp: %v", err)
		}
		if errors.Is(err, context.Canceled) {
			return err
		}
	} else {
		l.updatedLlamaCpp = true
	}

	return nil
}

// Run implements inference.Backend.Run.
func (l *llamaCpp) Run(ctx context.Context, socket, model string, mode inference.BackendMode, config *inference.BackendConfiguration) error {
	mdl, err := l.modelManager.GetModel(model)
	if err != nil {
		return fmt.Errorf("failed to get model: %w", err)
	}

	if err := os.RemoveAll(socket); err != nil && !errors.Is(err, fs.ErrNotExist) {
		l.log.Warnf("failed to remove socket file %s: %w\n", socket, err)
		l.log.Warnln("llama.cpp may not be able to start")
	}

	binPath := l.vendoredServerStoragePath
	if l.updatedLlamaCpp {
		binPath = l.updatedServerStoragePath
	}

	args, err := l.config.GetArgs(mdl, socket, mode, config)
	if err != nil {
		return fmt.Errorf("failed to get args for llama.cpp: %w", err)
	}

	l.log.Infof("llamaCppArgs: %v", args)
	llamaCppProcess := exec.CommandContext(
		ctx,
		filepath.Join(binPath, "com.docker.llama-server"),
		args...,
	)
	llamaCppProcess.Cancel = func() error {
		if runtime.GOOS == "windows" {
			return llamaCppProcess.Process.Kill()
		}
		return llamaCppProcess.Process.Signal(os.Interrupt)
	}
	tailBuf := tailbuffer.NewTailBuffer(1024)
	serverLogStream := l.serverLog.Writer()
	out := io.MultiWriter(serverLogStream, tailBuf)
	llamaCppProcess.Stdout = serverLogStream
	llamaCppProcess.Stderr = out

	if err := llamaCppProcess.Start(); err != nil {
		return fmt.Errorf("unable to start llama.cpp: %w", err)
	}

	llamaCppErrors := make(chan error, 1)
	go func() {
		llamaCppErr := llamaCppProcess.Wait()
		serverLogStream.Close()

		errOutput := new(strings.Builder)
		if _, err := io.Copy(errOutput, tailBuf); err != nil {
			l.log.Warnf("failed to read server output tail: %w", err)
		}

		if len(errOutput.String()) != 0 {
			llamaCppErr = fmt.Errorf("llama.cpp exit status: %w\nwith output: %s", llamaCppErr, errOutput.String())
		} else {
			llamaCppErr = fmt.Errorf("llama.cpp exit status: %w", llamaCppErr)
		}

		llamaCppErrors <- llamaCppErr
		close(llamaCppErrors)
		if err := os.Remove(socket); err != nil && !errors.Is(err, fs.ErrNotExist) {
			l.log.Warnf("failed to remove socket file %s on exit: %w\n", socket, err)
		}
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

func (l *llamaCpp) Status() string {
	return l.status
}

func (l *llamaCpp) GetDiskUsage() (int64, error) {
	size, err := diskusage.Size(l.updatedServerStoragePath)
	if err != nil {
		return 0, fmt.Errorf("error while getting store size: %v", err)
	}
	return size, nil
}

func (l *llamaCpp) GetRequiredMemoryForModel(model string, config *inference.BackendConfiguration) (*inference.RequiredMemory, error) {
	mdl, err := l.modelManager.GetModel(model)
	if err != nil {
		return nil, err
	}
	mdlPath, err := mdl.GGUFPath()
	if err != nil {
		return nil, err
	}
	mdlGguf, err := parser.ParseGGUFFile(mdlPath)
	if err != nil {
		return nil, err
	}
	mdlConfig, err := mdl.Config()
	if err != nil {
		return nil, err
	}

	contextSize := GetContextSize(&mdlConfig, config)

	// FIXME(p1-0tr): for now assume we are running on GPU (single one) - Devices[1];
	// sum up weights + kv cache + context for an estimate of total GPU memory needed
	// while running inference with the given model
	estimate := mdlGguf.EstimateLLaMACppRun(parser.WithLLaMACppContextSize(int32(contextSize)),
		// FIXME(p1-0tr): add logic for resolving other param values, instead of hardcoding them
		parser.WithLLaMACppLogicalBatchSize(2048),
		parser.WithLLaMACppOffloadLayers(100))
	memory := uint64(estimate.Devices[1].Weight.Sum() + estimate.Devices[1].KVCache.Sum() + estimate.Devices[1].Computation.Sum())

	return &inference.RequiredMemory{
		RAM:  0,
		VRAM: memory,
	}, nil
}
