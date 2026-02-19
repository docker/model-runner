package routing

import (
	"net/http"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends/vllmmetal"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/inference/scheduling"
	"github.com/docker/model-runner/pkg/logging"
	"github.com/docker/model-runner/pkg/metrics"
)

// BackendDef describes how to create a single inference backend.
type BackendDef struct {
	// Name is the key under which the backend will be registered.
	Name string
	// Init creates the backend. It receives the model manager, which
	// is not yet available when the BackendDef slice is constructed.
	Init func(*models.Manager) (inference.Backend, error)
}

// ServiceConfig holds the parameters needed to build the full inference
// service stack: model manager, model handler, scheduler, and router.
type ServiceConfig struct {
	Log          logging.Logger
	ClientConfig models.ClientConfig

	// Backends lists the backends to initialize. Each Init function
	// is called with the model manager during NewService.
	Backends []BackendDef

	// OnBackendError is called when a backend Init returns an error.
	// If nil, a warning is logged and the backend is skipped.
	OnBackendError func(name string, err error)

	// DefaultBackendName is the key used to look up the default backend
	// (typically llamacpp.Name).
	DefaultBackendName string

	// VLLMMetalServerPath is passed to vllmmetal.TryRegister. If empty
	// the default installation path is used.
	VLLMMetalServerPath string

	// HTTPClient is used by the scheduler for backend downloads and
	// health checks.
	HTTPClient *http.Client

	// MetricsTracker tracks inference metrics.
	MetricsTracker *metrics.Tracker

	// AllowedOrigins is forwarded to model, scheduler, Ollama, and
	// Anthropic handlers for CORS support. It may be nil.
	AllowedOrigins []string

	// ModelHandlerMiddleware optionally wraps the model handler before
	// route registration (e.g. for access restrictions).
	ModelHandlerMiddleware func(http.Handler) http.Handler

	// IncludeResponsesAPI enables the OpenAI Responses API compatibility
	// layer in the router.
	IncludeResponsesAPI bool

	// ExtraRoutes is called after the standard routes are registered.
	// The Service fields (except Router) are fully populated when this
	// is called, so the callback can reference them.
	ExtraRoutes func(*NormalizedServeMux, *Service)
}

// Service is the assembled inference service stack.
type Service struct {
	ModelManager  *models.Manager
	ModelHandler  *models.HTTPHandler
	Scheduler     *scheduling.Scheduler
	SchedulerHTTP *scheduling.HTTPHandler
	Router        *NormalizedServeMux
	Backends      map[string]inference.Backend
}

// NewService wires up the full inference service stack from the given
// configuration and returns the assembled Service.
func NewService(cfg ServiceConfig) *Service {
	modelManager := models.NewManager(cfg.Log, cfg.ClientConfig)
	modelHandler := models.NewHTTPHandler(cfg.Log, modelManager, cfg.AllowedOrigins)

	backends := initBackends(cfg.Log, modelManager, cfg.Backends, cfg.OnBackendError)
	deferredBackends := vllmmetal.TryRegister(cfg.Log, modelManager, backends, cfg.VLLMMetalServerPath)

	defaultBackend := backends[cfg.DefaultBackendName]

	scheduler := scheduling.NewScheduler(
		cfg.Log,
		backends,
		defaultBackend,
		modelManager,
		cfg.HTTPClient,
		cfg.MetricsTracker,
		deferredBackends,
	)

	schedulerHTTP := scheduling.NewHTTPHandler(scheduler, modelHandler, cfg.AllowedOrigins)

	svc := &Service{
		ModelManager:  modelManager,
		ModelHandler:  modelHandler,
		Scheduler:     scheduler,
		SchedulerHTTP: schedulerHTTP,
		Backends:      backends,
	}

	svc.Router = NewRouter(RouterConfig{
		Log:                    cfg.Log,
		Scheduler:              scheduler,
		SchedulerHTTP:          schedulerHTTP,
		ModelHandler:           modelHandler,
		ModelManager:           modelManager,
		AllowedOrigins:         cfg.AllowedOrigins,
		ModelHandlerMiddleware: cfg.ModelHandlerMiddleware,
		IncludeResponsesAPI:    cfg.IncludeResponsesAPI,
	})

	if cfg.ExtraRoutes != nil {
		cfg.ExtraRoutes(svc.Router, svc)
	}

	return svc
}

// initBackends creates and registers backends from the given definitions.
func initBackends(log logging.Logger, mm *models.Manager, defs []BackendDef, onError func(string, error)) map[string]inference.Backend {
	backends := make(map[string]inference.Backend, len(defs))
	for _, def := range defs {
		b, err := def.Init(mm)
		if err != nil {
			if onError != nil {
				onError(def.Name, err)
			} else {
				log.Warnf("unable to initialize %s backend: %v", def.Name, err)
			}
			continue
		}
		if b != nil {
			backends[def.Name] = b
		}
	}
	return backends
}
