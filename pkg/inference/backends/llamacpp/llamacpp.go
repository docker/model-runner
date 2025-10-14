package llamacpp

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/docker/model-runner/pkg/distribution/types"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	parser "github.com/gpustack/gguf-parser-go"

	"github.com/docker/model-runner/pkg/diskusage"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/config"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/logging"
	"github.com/docker/model-runner/pkg/sandbox"
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
	// gpuSupported indicates whether the underlying llama-server is built with GPU support.
	gpuSupported bool
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

	l.gpuSupported = l.checkGPUSupport(ctx)
	l.log.Infof("installed llama-server with gpuSupport=%t", l.gpuSupported)

	return nil
}

// Run implements inference.Backend.Run.
func (l *llamaCpp) Run(ctx context.Context, socket, model string, _ string, mode inference.BackendMode, config *inference.BackendConfiguration) error {
	bundle, err := l.modelManager.GetBundle(model)
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

	args, err := l.config.GetArgs(bundle, socket, mode, config)
	if err != nil {
		return fmt.Errorf("failed to get args for llama.cpp: %w", err)
	}

	l.log.Infof("llamaCppArgs: %v", args)
	tailBuf := tailbuffer.NewTailBuffer(1024)
	serverLogStream := l.serverLog.Writer()
	out := io.MultiWriter(serverLogStream, tailBuf)
	llamaCppSandbox, err := sandbox.Create(
		ctx,
		sandbox.ConfigurationLlamaCpp,
		func(command *exec.Cmd) {
			command.Cancel = func() error {
				if runtime.GOOS == "windows" {
					return command.Process.Kill()
				}
				return command.Process.Signal(os.Interrupt)
			}
			command.Stdout = serverLogStream
			command.Stderr = out
		},
		binPath,
		filepath.Join(binPath, "com.docker.llama-server"),
		args...,
	)
	if err != nil {
		return fmt.Errorf("unable to start llama.cpp: %w", err)
	}
	defer llamaCppSandbox.Close()

	llamaCppErrors := make(chan error, 1)
	go func() {
		llamaCppErr := llamaCppSandbox.Command().Wait()
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

func (l *llamaCpp) GetRequiredMemoryForModel(ctx context.Context, model string, config *inference.BackendConfiguration) (inference.RequiredMemory, error) {
	var mdlGguf *parser.GGUFFile
	var mdlConfig types.Config
	inStore, err := l.modelManager.IsModelInStore(model)
	if err != nil {
		return inference.RequiredMemory{}, fmt.Errorf("checking if model is in local store: %w", err)
	}
	if inStore {
		mdlGguf, mdlConfig, err = l.parseLocalModel(model)
		if err != nil {
			return inference.RequiredMemory{}, &inference.ErrGGUFParse{Err: err}
		}
	} else {
		mdlGguf, mdlConfig, err = l.parseRemoteModel(ctx, model)
		if err != nil {
			return inference.RequiredMemory{}, &inference.ErrGGUFParse{Err: err}
		}
	}

	contextSize := GetContextSize(mdlConfig, config)

	ngl := uint64(0)
	if l.gpuSupported {
		if runtime.GOOS == "windows" && runtime.GOARCH == "arm64" && mdlConfig.Quantization != "Q4_0" {
			ngl = 0 // only Q4_0 models can be accelerated on Adreno
		}
		ngl = 999
	}

	// TODO(p1-0tr): for now assume we are running on GPU (single one) - Devices[1];
	// sum up weights + kv cache + context for an estimate of total GPU memory needed
	// while running inference with the given model
	estimate := mdlGguf.EstimateLLaMACppRun(parser.WithLLaMACppContextSize(int32(contextSize)),
		// TODO(p1-0tr): add logic for resolving other param values, instead of hardcoding them
		parser.WithLLaMACppLogicalBatchSize(2048),
		parser.WithLLaMACppOffloadLayers(ngl))
	ram := uint64(estimate.Devices[0].Weight.Sum() + estimate.Devices[0].KVCache.Sum() + estimate.Devices[0].Computation.Sum())
	var vram uint64
	if len(estimate.Devices) > 1 {
		vram = uint64(estimate.Devices[1].Weight.Sum() + estimate.Devices[1].KVCache.Sum() + estimate.Devices[1].Computation.Sum())
	}

	if runtime.GOOS == "windows" && runtime.GOARCH == "arm64" {
		// TODO(p1-0tr): For now on windows/arm64 stick to the old behaviour, of allowing
		// one model at a time. This WA requires gpuinfo.GetVRAMSize to return 1.
		vram = 1
	}

	return inference.RequiredMemory{
		RAM:  ram,
		VRAM: vram,
	}, nil
}

func (l *llamaCpp) parseLocalModel(model string) (*parser.GGUFFile, types.Config, error) {
	bundle, err := l.modelManager.GetBundle(model)
	if err != nil {
		return nil, types.Config{}, fmt.Errorf("getting model(%s): %w", model, err)
	}
	modelGGUF, err := parser.ParseGGUFFile(bundle.GGUFPath())
	if err != nil {
		return nil, types.Config{}, fmt.Errorf("parsing gguf(%s): %w", bundle.GGUFPath(), err)
	}
	return modelGGUF, bundle.RuntimeConfig(), nil
}

func (l *llamaCpp) parseRemoteModel(ctx context.Context, model string) (*parser.GGUFFile, types.Config, error) {
	mdl, err := l.modelManager.GetRemoteModel(ctx, model)
	if err != nil {
		return nil, types.Config{}, fmt.Errorf("getting remote model(%s): %w", model, err)
	}
	layers, err := mdl.Layers()
	if err != nil {
		return nil, types.Config{}, fmt.Errorf("getting layers of model(%s): %w", model, err)
	}
	ggufLayers := getGGUFLayers(layers)
	if len(ggufLayers) != 1 {
		return nil, types.Config{}, fmt.Errorf(
			"remote memory estimation only supported for models with single GGUF layer, found %d layers", len(ggufLayers),
		)
	}
	ggufDigest, err := ggufLayers[0].Digest()
	if err != nil {
		return nil, types.Config{}, fmt.Errorf("getting digest of GGUF layer for model(%s): %w", model, err)
	}
	if ggufDigest.String() == "" {
		return nil, types.Config{}, fmt.Errorf("model(%s) has no GGUF layer", model)
	}
	blobURL, err := l.modelManager.GetRemoteModelBlobURL(model, ggufDigest)
	if err != nil {
		return nil, types.Config{}, fmt.Errorf("getting GGUF blob URL for model(%s): %w", model, err)
	}
	tok, err := l.modelManager.BearerTokenForModel(ctx, model)
	if err != nil {
		return nil, types.Config{}, fmt.Errorf("getting bearer token for model(%s): %w", model, err)
	}
	mdlGguf, err := parser.ParseGGUFFileRemote(ctx, blobURL, parser.UseBearerAuth(tok))
	if err != nil {
		return nil, types.Config{}, fmt.Errorf("parsing GGUF for model(%s): %w", model, err)
	}
	config, err := mdl.Config()
	if err != nil {
		return nil, types.Config{}, fmt.Errorf("getting config for model(%s): %w", model, err)
	}
	return mdlGguf, config, nil
}

func getGGUFLayers(layers []v1.Layer) []v1.Layer {
	var filtered []v1.Layer
	for _, layer := range layers {
		mt, err := layer.MediaType()
		if err != nil {
			continue
		}
		if mt == types.MediaTypeGGUF {
			filtered = append(filtered, layer)
		}
	}
	return filtered
}

func (l *llamaCpp) checkGPUSupport(ctx context.Context) bool {
	binPath := l.vendoredServerStoragePath
	if l.updatedLlamaCpp {
		binPath = l.updatedServerStoragePath
	}
	var output bytes.Buffer
	llamaCppSandbox, err := sandbox.Create(
		ctx,
		sandbox.ConfigurationLlamaCpp,
		func(command *exec.Cmd) {
			command.Stdout = &output
			command.Stderr = &output
		},
		filepath.Join(binPath, "com.docker.llama-server"),
		"--list-devices",
	)
	if err != nil {
		l.log.Warnf("Failed to start sandboxed llama.cpp process to probe GPU support: %v", err)
		return false
	}
	defer llamaCppSandbox.Close()
	if err := llamaCppSandbox.Command().Wait(); err != nil {
		l.log.Warnf("Failed to determine if llama-server is built with GPU support: %v", err)
		return false
	}
	sc := bufio.NewScanner(strings.NewReader(string(output.Bytes())))
	expectDev := false
	devRe := regexp.MustCompile(`\s{2}.*:\s`)
	ndevs := 0
	for sc.Scan() {
		if expectDev {
			if devRe.MatchString(sc.Text()) {
				ndevs += 1
			}
		} else {
			expectDev = strings.HasPrefix(sc.Text(), "Available devices:")
		}
	}
	return ndevs > 0
}
