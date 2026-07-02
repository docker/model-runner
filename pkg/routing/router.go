package routing

import (
	"net/http"

	"github.com/docker/model-runner/pkg/anthropic"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/inference/scheduling"
	"github.com/docker/model-runner/pkg/logging"
	"github.com/docker/model-runner/pkg/middleware"
	"github.com/docker/model-runner/pkg/ollama"
	"github.com/docker/model-runner/pkg/responses"
)

// RouterResult is the output of NewRouter, bundling the mux with any
// resources that require cleanup.
type RouterResult struct {
	Mux *NormalizedServeMux
	// closers holds cleanup functions that must be called when the
	// router is no longer needed (e.g. to stop the responses Store
	// background goroutine).
	closers []func()
}

// Close releases resources held by handlers registered on this router.
// It is idempotent and safe to call multiple times.
func (rr *RouterResult) Close() {
	for _, fn := range rr.closers {
		fn()
	}
	rr.closers = nil
}

// RouterConfig holds the dependencies needed to build the standard
// model-runner HTTP route structure.
type RouterConfig struct {
	Log           logging.Logger
	Scheduler     *scheduling.Scheduler
	SchedulerHTTP *scheduling.HTTPHandler
	ModelHandler  *models.HTTPHandler
	ModelManager  *models.Manager

	// AllowedOrigins is forwarded to the Ollama and Anthropic handlers
	// for CORS support. It may be nil.
	AllowedOrigins []string

	// ModelHandlerMiddleware optionally wraps the model handler before
	// registration (e.g. pinata uses this for access restrictions).
	// If nil the model handler is registered directly.
	ModelHandlerMiddleware func(http.Handler) http.Handler

	// IncludeResponsesAPI enables the OpenAI Responses API compatibility
	// layer, registering it under /responses, /v1/responses, and
	// /engines/responses prefixes. Requires SchedulerHTTP to be set.
	IncludeResponsesAPI bool
}

// NewRouter builds a NormalizedServeMux with the standard model-runner
// route structure: models endpoints, scheduler/inference endpoints,
// path aliases (/v1/, /rerank, /score), Ollama compatibility, and
// Anthropic compatibility.
//
// The returned RouterResult must be closed when the router is no longer
// needed to stop background goroutines (e.g. the responses Store cleanup).
func NewRouter(cfg RouterConfig) *RouterResult {
	router := NewNormalizedServeMux()
	result := &RouterResult{Mux: router}

	// Models endpoints – optionally wrapped by middleware.
	var modelEndpoint http.Handler = cfg.ModelHandler
	if cfg.ModelHandlerMiddleware != nil {
		modelEndpoint = cfg.ModelHandlerMiddleware(cfg.ModelHandler)
	}
	router.Handle(inference.ModelsPrefix+"/backend", cfg.SchedulerHTTP)
	router.Handle(inference.ModelsPrefix, modelEndpoint)
	router.Handle(inference.ModelsPrefix+"/", modelEndpoint)

	// Scheduler / inference endpoints.
	router.Handle(inference.InferencePrefix+"/", cfg.SchedulerHTTP)

	// Path aliases: /v1 → /engines/v1, /rerank → /engines/rerank, /score → /engines/score.
	aliasHandler := &middleware.AliasHandler{Handler: cfg.SchedulerHTTP}
	router.Handle("/v1/", aliasHandler)
	router.Handle("/rerank", aliasHandler)
	router.Handle("/score", aliasHandler)

	// Ollama API compatibility layer.
	ollamaHandler := ollama.NewHTTPHandler(cfg.Log, cfg.Scheduler, cfg.SchedulerHTTP, cfg.AllowedOrigins, cfg.ModelManager)
	router.Handle(ollama.APIPrefix+"/", ollamaHandler)

	// Anthropic Messages API compatibility layer.
	anthropicHandler := anthropic.NewHandler(cfg.Log, cfg.SchedulerHTTP, cfg.AllowedOrigins, cfg.ModelManager)
	router.Handle(anthropic.APIPrefix+"/", anthropicHandler)

	// OpenAI Responses API compatibility layer.
	if cfg.IncludeResponsesAPI {
		responsesHandler := responses.NewHTTPHandler(cfg.Log, cfg.SchedulerHTTP, cfg.AllowedOrigins)
		router.Handle(responses.APIPrefix+"/", responsesHandler)
		router.Handle(responses.APIPrefix, responsesHandler)
		router.Handle("/v1"+responses.APIPrefix+"/", responsesHandler)
		router.Handle("/v1"+responses.APIPrefix, responsesHandler)
		router.Handle(inference.InferencePrefix+responses.APIPrefix+"/", responsesHandler)
		router.Handle(inference.InferencePrefix+responses.APIPrefix, responsesHandler)
		result.closers = append(result.closers, responsesHandler.Close)
	}

	return result
}
