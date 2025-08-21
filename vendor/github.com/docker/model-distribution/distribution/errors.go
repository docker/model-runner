package distribution

import (
	"errors"
	"fmt"

	"github.com/docker/model-distribution/internal/store"
	"github.com/docker/model-distribution/registry"
	"github.com/docker/model-distribution/types"
)

var (
	ErrInvalidReference     = registry.ErrInvalidReference
	ErrModelNotFound        = store.ErrModelNotFound // model not found in store
	ErrUnsupportedMediaType = errors.New(fmt.Sprintf(
		"client supports only models of type %q and older - try upgrading",
		types.MediaTypeModelConfigV01,
	))
	ErrConflict = errors.New("resource conflict")
)

// ReferenceError represents an error related to an invalid model reference
type ReferenceError struct {
	Reference string
	Err       error
}

func (e *ReferenceError) Error() string {
	return fmt.Sprintf("invalid model reference %q: %v", e.Reference, e.Err)
}

func (e *ReferenceError) Unwrap() error {
	return e.Err
}

// Is implements error matching for ReferenceError
func (e *ReferenceError) Is(target error) bool {
	return target == ErrInvalidReference
}
