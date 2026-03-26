package distribution

import (
	"errors"

	"github.com/docker/model-runner/pkg/distribution/internal/store"
	"github.com/docker/model-runner/pkg/distribution/registry"
)

var (
	ErrInvalidReference = registry.ErrInvalidReference
	ErrModelNotFound    = store.ErrModelNotFound // model not found in store
	// ErrUnsupportedMediaType is returned when a model's config media type is
	// not supported by this client. The caller should wrap this with a dynamic
	// message that includes the actual and supported media types.
	ErrUnsupportedMediaType = errors.New("unsupported model config media type")
	ErrConflict             = errors.New("resource conflict")
)

const warnUnsupportedFormat = "vLLM backend currently only implemented for x86_64 NVIDIA platforms"
