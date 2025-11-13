package middleware

import (
	"net/http"

	"github.com/docker/model-runner/pkg/inference"
)

// V1AliasHandler provides an alias from /v1/ to /engines/v1/ paths
type V1AliasHandler struct {
	Handler http.Handler
}

func (h *V1AliasHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Clone the request with modified path: /v1/models -> /engines/v1/models
	r2 := r.Clone(r.Context())
	r2.URL.Path = inference.InferencePrefix + r.URL.Path

	h.Handler.ServeHTTP(w, r2)
}
