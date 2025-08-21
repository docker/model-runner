package registry

import (
	"errors"
	"fmt"

	"github.com/docker/model-distribution/types"
)

var (
	ErrInvalidReference     = errors.New("invalid model reference")
	ErrModelNotFound        = errors.New("model not found")
	ErrUnauthorized         = errors.New("unauthorized access to model")
	ErrUnsupportedMediaType = errors.New(fmt.Sprintf(
		"client supports only models of type %q and older - try upgrading",
		types.MediaTypeModelConfigV01,
	))
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

// Error represents an error returned by an OCI registry
type Error struct {
	Reference string
	// Code should be one of error codes defined in the distribution spec
	// (see https://github.com/opencontainers/distribution-spec/blob/583e014d15418d839d67f68152bc2c83821770e0/spec.md#error-codes)
	Code    string
	Message string
	Err     error
}

func (e Error) Error() string {
	return fmt.Sprintf("failed to pull model %q: %s - %s", e.Reference, e.Code, e.Message)
}

func (e Error) Unwrap() error {
	return e.Err
}

// Is implements error matching for Error
func (e Error) Is(target error) bool {
	switch target {
	case ErrModelNotFound:
		return e.Code == "MANIFEST_UNKNOWN" || e.Code == "NAME_UNKNOWN"
	case ErrUnauthorized:
		return e.Code == "UNAUTHORIZED"
	default:
		return false
	}
}

// NewReferenceError creates a new ReferenceError
func NewReferenceError(reference string, err error) error {
	return &ReferenceError{
		Reference: reference,
		Err:       err,
	}
}

// NewRegistryError creates a new Error
func NewRegistryError(reference, code, message string, err error) error {
	return &Error{
		Reference: reference,
		Code:      code,
		Message:   message,
		Err:       err,
	}
}
