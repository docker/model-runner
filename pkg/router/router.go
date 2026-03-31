//go:build cgo

// Package router provides the CGo bridge to the Rust dmr-router static
// library.  It exposes Start, which launches the axum HTTP router in a
// background goroutine and returns a StopFunc for graceful shutdown.
//
// The Rust library (router/libdmr_router.a) is compiled from router/src/lib.rs
// and linked at Go build time via the CGo directives below.
package router

/*
#cgo CFLAGS: -I${SRCDIR}/../../router
#cgo darwin LDFLAGS: -L${SRCDIR}/../../target/release -ldmr_router -framework Security -framework CoreFoundation -framework SystemConfiguration -lpthread -lresolv -ldl -lm
#cgo linux  LDFLAGS: -L${SRCDIR}/../../target/release -ldmr_router -lpthread -lresolv -ldl -lm
#include "dmr_router.h"
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"strings"
	"unsafe"
)

// Config holds the parameters passed to the Rust router.
type Config struct {
	// ListenSock is the Unix socket path the router listens on.
	// Leave empty to use ListenPort instead.
	ListenSock string
	// ListenPort is the TCP port to listen on when ListenSock is empty.
	ListenPort uint16

	// HandlerFn and HandlerCtx are the in-process Go handler callback
	// obtained from RegisterHandler.  When HandlerFn is non-nil requests
	// are dispatched directly to Go's http.Handler with no socket hop.
	// BackendSock and BackendPort are ignored in this mode.
	HandlerFn  unsafe.Pointer
	HandlerCtx unsafe.Pointer

	// BackendSock is the Unix socket path of the Go inference backend.
	// Used only when HandlerFn is nil.
	BackendSock string
	// BackendPort is the TCP port of the Go backend.
	// Used only when HandlerFn is nil and BackendSock is empty.
	BackendPort uint16

	// AllowedOrigins is a slice of allowed CORS origins.
	AllowedOrigins []string
	// Version is the version string returned by GET /version.
	Version string
}

// StopFunc stops the running router gracefully when called.
// It is safe to call from any goroutine and is idempotent.
type StopFunc func()

// Start launches the Rust HTTP router in a background goroutine.
//
// It returns a StopFunc and an error channel.  Call StopFunc to request a
// graceful shutdown.  The error channel receives exactly one value when the
// router exits: nil on clean shutdown, non-nil on error.
func Start(cfg Config) (StopFunc, <-chan error) {
	errCh := make(chan error, 1)

	// Pre-allocate the stop handle BEFORE spawning the goroutine.  This
	// eliminates the race where stopRouter() was called before Rust had
	// written handle_out (which only happens after block_on returns, i.e.
	// after the router has already shut down — making stop a no-op and
	// leaving the process hung waiting for the router to exit).
	//
	// Rust wires the oneshot sender into this handle at the start of
	// dmr_router_serve, before the event loop blocks.
	handle := C.dmr_router_new_handle()

	// Build C strings. They are freed inside the goroutine after
	// dmr_router_serve returns (i.e. after the router has shut down).
	var cListenSock, cBackendSock, cOrigins, cVersion *C.char
	if cfg.ListenSock != "" {
		cListenSock = C.CString(cfg.ListenSock)
	}
	if cfg.BackendSock != "" {
		cBackendSock = C.CString(cfg.BackendSock)
	}
	if len(cfg.AllowedOrigins) > 0 {
		cOrigins = C.CString(strings.Join(cfg.AllowedOrigins, ","))
	}
	cVersion = C.CString(cfg.Version)

	ccfg := C.DmrRouterConfig{
		listen_sock:     cListenSock,
		listen_port:     C.uint16_t(cfg.ListenPort),
		handler_fn:      (*[0]byte)(cfg.HandlerFn),
		handler_ctx:     cfg.HandlerCtx,
		backend_sock:    cBackendSock,
		backend_port:    C.uint16_t(cfg.BackendPort),
		allowed_origins: cOrigins,
		version:         cVersion,
	}

	go func() {
		// Pass the pre-allocated handle; Rust wires stop_tx into it before
		// blocking, so dmr_router_stop can be called at any point.
		rc := C.dmr_router_serve(&ccfg, &handle)

		// Free C strings now that the blocking call has returned.
		if cListenSock != nil {
			C.free(unsafe.Pointer(cListenSock))
		}
		if cBackendSock != nil {
			C.free(unsafe.Pointer(cBackendSock))
		}
		if cOrigins != nil {
			C.free(unsafe.Pointer(cOrigins))
		}
		C.free(unsafe.Pointer(cVersion))

		if rc != 0 {
			errCh <- fmt.Errorf("dmr-router exited with code %d", int(rc))
		} else {
			errCh <- nil
		}
	}()

	stop := func() {
		// handle was pre-allocated before the goroutine started, and Rust
		// wired the oneshot sender into it before blocking — so this is
		// always safe to call, even immediately after Start returns.
		C.dmr_router_stop(handle)
	}

	return stop, errCh
}
