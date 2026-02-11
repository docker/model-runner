package scheduling

import (
	"fmt"
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/logging"
)

var (
	// errInstallerNotStarted indicates that the installer has not yet been
	// started and thus installation waits are not possible.
	errInstallerNotStarted = errors.New("backend installer not started")
	// errInstallerShuttingDown indicates that the installer's run loop has been
	// terminated and the installer is shutting down.
	errInstallerShuttingDown = errors.New("backend installer shutting down")
	// errBackendNotInstalled indicates that a deferred backend has not been
	// installed. Callers should install it via installBackend before use.
	errBackendNotInstalled = errors.New("backend not installed")
)

// installStatus tracks the installation status of a backend.
type installStatus struct {
	// installed is closed if and when the corresponding backend's installation
	// completes successfully.
	installed chan struct{}
	// failed is closed if the corresponding backend's installation fails. If
	// this channel is closed, then err can be read and returned.
	failed chan struct{}
	// err is the error that occurred during installation. It should only be
	// accessed by readers if (and after) failed is closed.
	err error
}

// installer drives backend installations.
type installer struct {
	// log is the associated logger.
	log logging.Logger
	// backends are the supported inference backends.
	backends map[string]inference.Backend
	// httpClient is the HTTP client to use for backend installations.
	httpClient *http.Client
	// started tracks whether or not the installer has been started.
	started atomic.Bool
	// statuses maps backend names to their installation statuses.
	statuses map[string]*installStatus
	// deferredBackends tracks backends whose installation is deferred until
	// explicitly requested via installBackend.
	deferredBackends map[string]bool
	// mu protects statuses map mutations in installBackend. Readers
	// (wait, isInstalled) take an RLock; installBackend takes a full Lock.
	mu sync.RWMutex
	// installMu serializes on-demand install operations so that only one
	// goroutine performs the actual download at a time. Held independently
	// of mu so that long-running installs don't block map readers.
	installMu sync.Mutex
}

// newInstaller creates a new backend installer. Backends listed in
// deferredBackends are skipped during the automatic run loop and must be
// installed on-demand via installBackend.
func newInstaller(
	log logging.Logger,
	backends map[string]inference.Backend,
	httpClient *http.Client,
	deferredBackends []string,
) *installer {
	// Build the deferred set.
	deferred := make(map[string]bool, len(deferredBackends))
	for _, name := range deferredBackends {
		deferred[name] = true
	}

	// Create status trackers.
	statuses := make(map[string]*installStatus, len(backends))
	for name := range backends {
		statuses[name] = &installStatus{
			installed: make(chan struct{}),
			failed:    make(chan struct{}),
		}
	}

	// Create the installer.
	return &installer{
		log:              log,
		backends:         backends,
		httpClient:       httpClient,
		statuses:         statuses,
		deferredBackends: deferred,
	}
}

// run is the main run loop for the installer.
func (i *installer) run(ctx context.Context) {
	// Mark the installer as having started.
	i.started.Store(true)

	// Attempt to install each backend and update statuses.
	//
	// TODO: We may want to add a backoff + retry mechanism.
	//
	// TODO: We currently try to install all known backends. We may wish to add
	// granular, per-backend settings. For now, with llama.cpp as our only
	// ubiquitous backend and mlx as a relatively lightweight backend (on macOS
	// only), this granularity is probably less of a concern.
	for name, backend := range i.backends {
		// For deferred backends, check if they are already installed on disk
		// from a previous session. Only call Install() (which verifies the
		// existing installation) when files are present, to avoid triggering
		// a download.
		if i.deferredBackends[name] {
			status := i.statuses[name]
			if diskUsage, err := backend.GetDiskUsage(); err == nil && diskUsage > 0 {
				if err := backend.Install(ctx, i.httpClient); err != nil {
					status.err = err
					close(status.failed)
				} else {
					close(status.installed)
				}
			}
			// If not on disk, leave channels open so wait() can trigger
			// on-demand installation when the backend is first needed.
			continue
		}

		status := i.statuses[name]

		var installedClosed bool
		select {
		case <-status.installed:
			installedClosed = true
		default:
			installedClosed = false
		}

		if (status.err != nil && !errors.Is(status.err, context.Canceled)) || installedClosed {
			continue
		}
		if err := backend.Install(ctx, i.httpClient); err != nil {
			i.log.Warn(fmt.Sprintf("Backend installation failed for %s: %v", name, err))
			select {
			case <-ctx.Done():
				status.err = errors.Join(errInstallerShuttingDown, ctx.Err())
				continue
			default:
				status.err = err
			}
			close(status.failed)
		} else {
			close(status.installed)
		}
	}
}

// wait waits for installation of the specified backend to complete or fail.
// For deferred backends that have not yet been installed, it triggers
// on-demand installation (auto-pull), blocking until complete or the caller's
// context is cancelled.
func (i *installer) wait(ctx context.Context, backend string) error {
	// Grab the backend status under a read lock, since installBackend may replace entries in the map.
	i.mu.RLock()
	status, ok := i.statuses[backend]
	i.mu.RUnlock()
	if !ok {
		return ErrBackendNotFound
	}

	// For deferred backends, check whether installation has already completed.
	// If not, trigger on-demand installation (auto-pull).
	if i.deferredBackends[backend] {
		select {
		case <-status.installed:
			return nil
		case <-status.failed:
			return status.err
		default:
			return i.installBackend(ctx, backend)
		}
	}

	// If the installer hasn't started, then don't poll for readiness, because
	// it may never come. If it has started, then even if it's cancelled we can
	// be sure that we'll at least see failure for all backend installations.
	if !i.started.Load() {
		return errInstallerNotStarted
	}

	// Wait for readiness.
	select {
	case <-ctx.Done():
		return context.Canceled
	case <-status.installed:
		return nil
	case <-status.failed:
		return status.err
	}
}

// installBackend triggers on-demand installation of a deferred backend.
// It is idempotent: if the backend is already installed, it returns nil.
// installMu serializes actual downloads so only one goroutine installs at a
// time, while mu is held only briefly for map reads/writes so that other
// goroutines calling wait() or isInstalled() are not blocked during the
// (potentially long) Install() call.
func (i *installer) installBackend(ctx context.Context, name string) error {
	// Serialize install operations so only one download runs at a time.
	i.installMu.Lock()
	defer i.installMu.Unlock()

	backend, ok := i.backends[name]
	if !ok {
		return ErrBackendNotFound
	}

	// Check current status under read lock.
	i.mu.RLock()
	status := i.statuses[name]
	i.mu.RUnlock()

	// Already installed — nothing to do.
	select {
	case <-status.installed:
		return nil
	default:
	}

	// If previously failed, reset status for retry.
	select {
	case <-status.failed:
		status = &installStatus{
			installed: make(chan struct{}),
			failed:    make(chan struct{}),
		}
		i.mu.Lock()
		i.statuses[name] = status
		i.mu.Unlock()
	default:
	}

	// Perform installation without holding mu.
	if err := backend.Install(ctx, i.httpClient); err != nil {
		// If the caller's context was cancelled (e.g. Ctrl-C), don't
		// permanently mark the backend as failed — leave channels open
		// so the next request can retry.
		if ctx.Err() != nil {
			return err
		}
		status.err = err
		close(status.failed)
		return err
	}

	close(status.installed)
	return nil
}

// isInstalled returns true if the given backend has completed installation.
// It is non-blocking.
func (i *installer) isInstalled(name string) bool {
	i.mu.RLock()
	status, ok := i.statuses[name]
	i.mu.RUnlock()
	if !ok {
		return false
	}
	select {
	case <-status.installed:
		return true
	default:
		return false
	}
}
