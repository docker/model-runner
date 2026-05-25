package routing

import (
	"log/slog"
	"testing"
)

// TestNewRouter_WithResponsesAPI_Close verifies that creating a router with
// IncludeResponsesAPI enabled and then calling Close does not leak
// goroutines. The goleak detector in TestMain will catch any leak.
func TestNewRouter_WithResponsesAPI_Close(t *testing.T) {
	log := slog.New(slog.DiscardHandler)

	result := NewRouter(RouterConfig{
		Log:                 log,
		IncludeResponsesAPI: true,
		// Scheduler, ModelHandler, etc. are nil — the responses handler
		// only needs them when actually serving requests, not for route
		// registration and cleanup.
	})

	// Verify the mux was created.
	if result.Mux == nil {
		t.Fatal("expected non-nil Mux")
	}

	// Close must stop the responses Store cleanup goroutine.
	result.Close()
}

// TestNewRouter_WithoutResponsesAPI_Close verifies that Close is safe
// to call even when the Responses API is not enabled (no closers).
func TestNewRouter_WithoutResponsesAPI_Close(t *testing.T) {
	log := slog.New(slog.DiscardHandler)

	result := NewRouter(RouterConfig{
		Log:                 log,
		IncludeResponsesAPI: false,
	})

	if result.Mux == nil {
		t.Fatal("expected non-nil Mux")
	}

	// Should be a no-op, must not panic.
	result.Close()
}
