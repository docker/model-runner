//go:build !cgo

// Package router provides a stub implementation of the Rust dmr-router bridge
// for environments where CGo is disabled (e.g. lint passes, cross-compilation
// targets that do not support the static library).
//
// The real implementation lives in router.go and handler.go and requires CGo.
package router

import (
	"fmt"
	"net/http"
	"unsafe"
)

// Config holds the parameters passed to the Rust router.
type Config struct {
	ListenSock     string
	ListenPort     uint16
	HandlerFn      unsafe.Pointer
	HandlerCtx     unsafe.Pointer
	BackendSock    string
	BackendPort    uint16
	AllowedOrigins []string
	Version        string
}

// StopFunc stops the running router gracefully when called.
type StopFunc func()

// Start is a stub that returns an error immediately when CGo is disabled.
func Start(_ Config) (StopFunc, <-chan error) {
	errCh := make(chan error, 1)
	errCh <- fmt.Errorf("dmr-router: CGo is required but was disabled at build time")
	return func() {}, errCh
}

// RegisterHandler is a stub that returns nil pointers when CGo is disabled.
func RegisterHandler(_ http.Handler) (handlerFn unsafe.Pointer, handlerCtx unsafe.Pointer) {
	return nil, nil
}
