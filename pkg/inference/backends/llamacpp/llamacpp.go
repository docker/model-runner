package llamacpp

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/docker/model-runner/pkg/diskusage"
	"github.com/docker/model-runner/pkg/distribution/oci"
	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends"
	"github.com/docker/model-runner/pkg/inference/config"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/logging"
	"github.com/docker/model-runner/pkg/sandbox"
	parser "github.com/gpustack/gguf-parser-go"
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

// UsesTCP implements inference.Backend.UsesTCP.
func (l *llamaCpp) UsesTCP() bool {
	return false
}

// Install implements inference.Backend.Install.
func (l *llamaCpp) Install(ctx context.Context, httpClient *http.Client) error {
	l.updatedLlamaCpp = false

	// We don't currently support this backend on Windows. We'll likely
	// never support it on Intel Macs.
	if (runtime.GOOS == "darwin" && runtime.GOARCH == "amd64") ||
		(runtime.GOOS == "windows" && runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
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
		if !errors.Is(err, errLlamaCppUpToDate) && !errors.Is(err, errLlamaCppUpdateDisabled) {
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

	var draftBundle types.ModelBundle
	if config != nil && config.Speculative != nil && config.Speculative.DraftModel != "" {
		draftBundle, err = l.modelManager.GetBundle(config.Speculative.DraftModel)
		if err != nil {
			return fmt.Errorf("failed to get draft model: %w", err)
		}
	}

	binPath := l.vendoredServerStoragePath
	if l.updatedLlamaCpp {
		binPath = l.updatedServerStoragePath
	}

	args, err := l.config.GetArgs(bundle, socket, mode, config)
	if err != nil {
		return fmt.Errorf("failed to get args for llama.cpp: %w", err)
	}

	if draftBundle != nil && config != nil && config.Speculative != nil {
		draftPath := draftBundle.GGUFPath()
		if draftPath != "" {
			args = append(args, "--model-draft", draftPath)
			if config.Speculative.NumTokens > 0 {
				args = append(args, "--draft-max", strconv.Itoa(config.Speculative.NumTokens))
			}
			if config.Speculative.MinAcceptanceRate > 0 {
				args = append(args, "--draft-p-min", strconv.FormatFloat(config.Speculative.MinAcceptanceRate, 'f', 2, 64))
			}
		}
	}

	return backends.RunBackend(ctx, backends.RunnerConfig{
		BackendName:     "llama.cpp",
		Socket:          socket,
		BinaryPath:      filepath.Join(binPath, "com.docker.llama-server"),
		SandboxPath:     binPath,
		SandboxConfig:   sandbox.ConfigurationLlamaCpp,
		Args:            args,
		Logger:          l.log,
		ServerLogWriter: l.serverLog.Writer(),
	})
}

func (l *llamaCpp) Status() string {
	return l.status
}

func (l *llamaCpp) GetDiskUsage() (int64, error) {
	size, err := diskusage.Size(l.updatedServerStoragePath)
	if err != nil {
		return 0, fmt.Errorf("error while getting store size: %w", err)
	}
	return size, nil
}

func (l *llamaCpp) GetRequiredMemoryForModel(ctx context.Context, model string, config *inference.BackendConfiguration) (inference.RequiredMemory, error) {
	mdlGguf, mdlConfig, err := l.parseModel(ctx, model)
	if err != nil {
		return inference.RequiredMemory{}, &inference.ErrGGUFParse{Err: err}
	}

	configuredContextSize := GetContextSize(mdlConfig, config)
	contextSize := int32(4096) // default context size
	if configuredContextSize != nil {
		contextSize = *configuredContextSize
	}

	var ngl uint64
	if l.gpuSupported {
		ngl = 999
		if runtime.GOOS == "windows" && runtime.GOARCH == "arm64" && mdlConfig.GetQuantization() != "Q4_0" {
			ngl = 0 // only Q4_0 models can be accelerated on Adreno
		}
	}

	memory := l.estimateMemoryFromGGUF(mdlGguf, contextSize, ngl)

	if config != nil && config.Speculative != nil && config.Speculative.DraftModel != "" {
		draftGguf, _, err := l.parseModel(ctx, config.Speculative.DraftModel)
		if err != nil {
			return inference.RequiredMemory{}, fmt.Errorf("estimating draft model memory: %w", &inference.ErrGGUFParse{Err: err})
		}
		draftMemory := l.estimateMemoryFromGGUF(draftGguf, contextSize, ngl)
		memory.RAM += draftMemory.RAM
		memory.VRAM += draftMemory.VRAM
	}

	if runtime.GOOS == "windows" && runtime.GOARCH == "arm64" {
		memory.VRAM = 1
	}

	return memory, nil
}

// parseModel parses a model (local or remote) and returns the GGUF file and config.
func (l *llamaCpp) parseModel(ctx context.Context, model string) (*parser.GGUFFile, types.ModelConfig, error) {
	inStore, err := l.modelManager.InStore(model)
	if err != nil {
		return nil, nil, fmt.Errorf("checking if model is in local store: %w", err)
	}
	if inStore {
		return l.parseLocalModel(model)
	}
	return l.parseRemoteModel(ctx, model)
}

// estimateMemoryFromGGUF estimates memory requirements from a parsed GGUF file.
func (l *llamaCpp) estimateMemoryFromGGUF(ggufFile *parser.GGUFFile, contextSize int32, ngl uint64) inference.RequiredMemory {
	estimate := ggufFile.EstimateLLaMACppRun(
		parser.WithLLaMACppContextSize(contextSize),
		parser.WithLLaMACppLogicalBatchSize(2048),
		parser.WithLLaMACppOffloadLayers(ngl),
	)
	ram := uint64(estimate.Devices[0].Weight.Sum() + estimate.Devices[0].KVCache.Sum() + estimate.Devices[0].Computation.Sum())
	var vram uint64
	if len(estimate.Devices) > 1 {
		vram = uint64(estimate.Devices[1].Weight.Sum() + estimate.Devices[1].KVCache.Sum() + estimate.Devices[1].Computation.Sum())
	}

	return inference.RequiredMemory{
		RAM:  ram,
		VRAM: vram,
	}
}

func (l *llamaCpp) parseLocalModel(model string) (*parser.GGUFFile, types.ModelConfig, error) {
	bundle, err := l.modelManager.GetBundle(model)
	if err != nil {
		return nil, nil, fmt.Errorf("getting model(%s): %w", model, err)
	}
	modelGGUF, err := parser.ParseGGUFFile(bundle.GGUFPath())
	if err != nil {
		return nil, nil, fmt.Errorf("parsing gguf(%s): %w", bundle.GGUFPath(), err)
	}
	return modelGGUF, bundle.RuntimeConfig(), nil
}

func (l *llamaCpp) parseRemoteModel(ctx context.Context, model string) (*parser.GGUFFile, types.ModelConfig, error) {
	mdl, err := l.modelManager.GetRemote(ctx, model)
	if err != nil {
		return nil, nil, fmt.Errorf("getting remote model(%s): %w", model, err)
	}
	layers, err := mdl.Layers()
	if err != nil {
		return nil, nil, fmt.Errorf("getting layers of model(%s): %w", model, err)
	}
	ggufLayers := getGGUFLayers(layers)
	if len(ggufLayers) != 1 {
		return nil, nil, fmt.Errorf(
			"remote memory estimation only supported for models with single GGUF layer, found %d layers", len(ggufLayers),
		)
	}
	ggufDigest, err := ggufLayers[0].Digest()
	if err != nil {
		return nil, nil, fmt.Errorf("getting digest of GGUF layer for model(%s): %w", model, err)
	}
	if ggufDigest.String() == "" {
		return nil, nil, fmt.Errorf("model(%s) has no GGUF layer", model)
	}
	blobURL, err := l.modelManager.GetRemoteBlobURL(model, ggufDigest)
	if err != nil {
		return nil, nil, fmt.Errorf("getting GGUF blob URL for model(%s): %w", model, err)
	}
	tok, err := l.modelManager.BearerTokenForModel(ctx, model)
	if err != nil {
		return nil, nil, fmt.Errorf("getting bearer token for model(%s): %w", model, err)
	}
	mdlGguf, err := parser.ParseGGUFFileRemote(ctx, blobURL, parser.UseBearerAuth(tok))
	if err != nil {
		return nil, nil, fmt.Errorf("parsing GGUF for model(%s): %w", model, err)
	}
	config, err := mdl.Config()
	if err != nil {
		return nil, nil, fmt.Errorf("getting config for model(%s): %w", model, err)
	}
	return mdlGguf, config, nil
}

func getGGUFLayers(layers []oci.Layer) []oci.Layer {
	var filtered []oci.Layer
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
		binPath,
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
	sc := bufio.NewScanner(strings.NewReader(output.String()))
	expectDev := false
	devRe := regexp.MustCompile(`\s{2}.*:\s`)
	ndevs := 0
	for sc.Scan() {
		if expectDev {
			if devRe.MatchString(sc.Text()) {
				ndevs++
			}
		} else {
			expectDev = strings.HasPrefix(sc.Text(), "Available devices:")
		}
	}
	return ndevs > 0
}
