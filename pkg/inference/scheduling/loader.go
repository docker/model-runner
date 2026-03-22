package scheduling

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"time"

	"github.com/docker/model-runner/pkg/environment"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/internal/utils"
	"github.com/docker/model-runner/pkg/logging"
	"github.com/docker/model-runner/pkg/metrics"
)

const (
	// maximumRunnerSlots is the maximum number of runner slots allowed.
	// TODO: We may wish to make this a tunable option, though for the time
	// being it is almost certainly greater than the number of models that most
	// developers' systems will be able to load.
	maximumRunnerSlots = 8
	// defaultRunnerIdleTimeout is the default maximum amount of time that a
	// runner can sit idle (i.e. without any requests) before being terminated.
	defaultRunnerIdleTimeout = 5 * time.Minute
)

var (
	// errLoadsDisabled indicates that backend loads are currently disabled.
	errLoadsDisabled = errors.New("backend loading disabled")
	// errRunnerAlreadyActive indicates that a given runner is already active
	// and therefore can't be reconfigured for example
	errRunnerAlreadyActive = errors.New("runner already active")
)

// runnerKey is used to index runners.
type runnerKey struct {
	// backend is the backend associated with the runner.
	backend string
	// modelID is the ID (digest) of the model associated with the runner.
	modelID string
	// draftModelID is the ID (digest) of the draft model for speculative decoding (empty if not used).
	draftModelID string
	// mode is the operation mode associated with the runner.
	mode inference.BackendMode
}

// makeConfigKey creates a runnerKey for configuration storage.
// Configuration keys always use an empty draftModelID since the draft model
// is specified within the configuration itself, not as part of the key.
func makeConfigKey(backendName, modelID string, mode inference.BackendMode) runnerKey {
	return runnerKey{
		backend:      backendName,
		modelID:      modelID,
		draftModelID: "",
		mode:         mode,
	}
}

// makeRunnerKey creates a runnerKey for runner registration and lookup.
// Runner keys include the draftModelID to uniquely identify runners with
// different speculative decoding configurations.
func makeRunnerKey(backendName, modelID, draftModelID string, mode inference.BackendMode) runnerKey {
	return runnerKey{
		backend:      backendName,
		modelID:      modelID,
		draftModelID: draftModelID,
		mode:         mode,
	}
}

// runnerInfo holds information about a runner including its slot and the original model reference used to load it.
type runnerInfo struct {
	// slot is the slot index where the runner is stored.
	slot int
	// modelRef is the original model reference (tag) used to load the runner.
	modelRef string
}

// loadingInfo holds metadata about a runner that is being initialized.
type loadingInfo struct {
	backendName  string
	modelID      string
	draftModelID string
	modelRef     string
	mode         inference.BackendMode
}

// loader manages the loading and unloading of backend runners. It regulates
// active backends in a manner that avoids exhausting system resources. Loaders
// assume that all of their backends have been installed, so no load requests
// should be made until the caller is certain that the corresponding backend has
// been installed successfully.
type loader struct {
	// log is the associated logger.
	log logging.Logger
	// backends are the supported inference backends.
	backends map[string]inference.Backend
	// modelManager is the shared model manager.
	modelManager *models.Manager
	// runnerIdleTimeout is the loader-specific default runner idle timeout.
	runnerIdleTimeout time.Duration
	// idleCheck is used to signal the run loop when timestamps have updated.
	idleCheck chan struct{}
	// guard is a sempahore controlling access to all subsequent fields. It is
	// buffered (with size 1) and contains a single element that must be held in
	// order to operate on those fields. We use a channel (instead of a
	// sync.Mutex) to enable polling.
	guard chan struct{}
	// loadsEnabled signals that loads are currently enabled.
	loadsEnabled bool
	// waiters is the set of signal channels associated with waiting loaders. We
	// use a set of signaling channels (instead of a sync.Cond) to enable
	// polling. Each signaling channel should be buffered (with size 1).
	waiters map[chan<- struct{}]bool
	// runners maps runner keys to their slot index.
	runners map[runnerKey]runnerInfo
	// loading tracks slots that have a runner being initialized. This
	// allows the lock to be released during long-running operations
	// (run + wait) while still preventing other goroutines from using
	// or evicting those slots. The value contains metadata needed to
	// report loading status.
	loading map[int]loadingInfo
	// slots maps slot indices to associated runners. A slot is considered free
	// if the runner value in it is nil.
	slots []*runner
	// references maps slot indices to reference counts.
	references []uint
	// timestamps maps slot indices to last usage times. Values in this slice
	// are only valid if the corresponding reference count is zero.
	timestamps []time.Time
	// runnerConfigs maps model names to runner configurations
	runnerConfigs map[runnerKey]inference.BackendConfiguration
	// openAIRecorder is used to record OpenAI API inference requests and responses.
	openAIRecorder *metrics.OpenAIRecorder
}

// newLoader creates a new loader.
func newLoader(
	log logging.Logger,
	backends map[string]inference.Backend,
	modelManager *models.Manager,
	openAIRecorder *metrics.OpenAIRecorder,
) *loader {
	// Compute the number of runner slots to allocate. Because of RAM and VRAM
	// limitations, it's unlikely that we'll ever be able to fully populate
	// these slots, so for now we just choose a reasonable value. We may need to
	// tune this heuristic for systems with enormous amounts of VRAM.
	nSlots := min(runtime.NumCPU(), maximumRunnerSlots)

	// Check if we have a special environment.
	isGPUEnabledCloudEnvironment := environment.Get() == environment.EnvironmentCloud &&
		os.Getenv("NVIDIA_VISIBLE_DEVICES") != ""

	// Compute the default idle runner timeout. Per-model keep_alive
	// overrides this via runnerIdleTimeoutFor().
	runnerIdleTimeout := defaultRunnerIdleTimeout
	if isGPUEnabledCloudEnvironment {
		runnerIdleTimeout = 8 * time.Hour
	}

	// Create the loader.
	l := &loader{
		log:               log,
		backends:          backends,
		modelManager:      modelManager,
		runnerIdleTimeout: runnerIdleTimeout,
		idleCheck:         make(chan struct{}, 1),
		guard:             make(chan struct{}, 1),
		waiters:           make(map[chan<- struct{}]bool),
		runners:           make(map[runnerKey]runnerInfo, nSlots),
		loading:           make(map[int]loadingInfo),
		slots:             make([]*runner, nSlots),
		references:        make([]uint, nSlots),
		timestamps:        make([]time.Time, nSlots),
		runnerConfigs:     make(map[runnerKey]inference.BackendConfiguration),
		openAIRecorder:    openAIRecorder,
	}
	l.guard <- struct{}{}
	return l
}

// lock acquires the guard semaphore. It returns true if the lock was acquired
// and false if ctx is cancelled before acquisition.
func (l *loader) lock(ctx context.Context) bool {
	select {
	case <-l.guard:
		return true
	case <-ctx.Done():
		return false
	}
}

// unlock releases the guard semaphore.
func (l *loader) unlock() {
	l.guard <- struct{}{}
}

// broadcast signals all waiters. Callers must hold the loader lock.
func (l *loader) broadcast() {
	for waiter := range l.waiters {
		select {
		case waiter <- struct{}{}:
		default:
		}
	}
}

// freeRunnerSlot frees a runner slot.
// The caller must hold the loader lock.
func (l *loader) freeRunnerSlot(slot int, key runnerKey) {
	l.slots[slot].terminate()
	l.slots[slot] = nil
	l.timestamps[slot] = time.Time{}
	delete(l.runners, key)
}

// runnerIdleTimeoutFor returns the idle timeout for a runner, using its
// per-model keep_alive if configured, otherwise the loader default.
// A negative return value means never idle-evict.
func (l *loader) runnerIdleTimeoutFor(r runnerKey) time.Duration {
	configKey := makeConfigKey(r.backend, r.modelID, r.mode)
	if cfg, ok := l.runnerConfigs[configKey]; ok && cfg.KeepAlive != nil {
		return cfg.KeepAlive.Duration()
	}
	return l.runnerIdleTimeout
}

// evict evicts all unused runners from the loader. If idleOnly is true, then
// only those unused, but functioning, runners which are considered "idle" (based
// on usage timestamp) are evicted. Defunct (e.g. crashed) runners will be evicted
// regardless of whether they are considered "idle". The caller must hold the loader
// lock. It returns the number of remaining runners.
func (l *loader) evict(idleOnly bool) int {
	now := time.Now()
	evictedCount := 0
	for r, runnerInfo := range l.runners {
		unused := l.references[runnerInfo.slot] == 0
		timeout := l.runnerIdleTimeoutFor(r)
		neverEvict := timeout < 0
		idle := unused && !neverEvict && now.Sub(l.timestamps[runnerInfo.slot]) > timeout
		defunct := false
		select {
		case <-l.slots[runnerInfo.slot].done:
			defunct = true
		default:
		}
		if unused && (!idleOnly || idle || defunct) && (!idleOnly || !neverEvict || defunct) {
			l.log.Info("Evicting backend runner", "backend", r.backend, "model", r.modelID, "modelRef", runnerInfo.modelRef, "mode", r.mode)
			l.freeRunnerSlot(runnerInfo.slot, r)
			evictedCount++
		} else if unused {
			l.log.Debug("Runner is unused but not evictable", "modelID", r.modelID, "modelRef", runnerInfo.modelRef, "idleOnly", idleOnly, "idle", idle, "defunct", defunct, "neverEvict", neverEvict)
		} else {
			l.log.Debug("Runner is in use with references, cannot evict", "modelID", r.modelID, "modelRef", runnerInfo.modelRef, "references", l.references[runnerInfo.slot])
		}
	}
	if evictedCount > 0 {
		l.log.Info("Evicted runner(s)", "count", evictedCount)
	}
	return len(l.runners)
}

// evictRunner evicts a specific runner. The caller must hold the loader lock.
// It returns the number of remaining runners.
func (l *loader) evictRunner(backend, model string, mode inference.BackendMode) int {
	allBackends := backend == ""
	found := false
	for r, runnerInfo := range l.runners {
		unused := l.references[runnerInfo.slot] == 0
		if unused && (allBackends || r.backend == backend) && r.modelID == model && r.mode == mode {
			l.log.Info("Evicting backend runner", "backend", r.backend, "model", r.modelID, "modelRef", runnerInfo.modelRef, "mode", r.mode)
			l.freeRunnerSlot(runnerInfo.slot, r)
			found = true
		}
	}
	if !found {
		l.log.Warn("No unused runner found", "backend", utils.SanitizeForLog(backend), "model", utils.SanitizeForLog(model), "mode", utils.SanitizeForLog(string(mode)))
	}
	return len(l.runners)
}

// UnloadBackend unloads all runners for a specific backend.
// It returns the number of unloaded runners.
func (l *loader) UnloadBackend(ctx context.Context, backend string) int {
	if !l.lock(ctx) {
		return 0
	}
	defer l.unlock()

	count := 0
	for r, runnerInfo := range l.runners {
		if r.backend == backend && l.references[runnerInfo.slot] == 0 {
			l.log.Info("Evicting backend runner for uninstall", "backend", r.backend, "model", r.modelID, "modelRef", runnerInfo.modelRef, "mode", r.mode)
			l.freeRunnerSlot(runnerInfo.slot, r)
			count++
		}
	}
	return count
}

// Unload unloads runners and returns the number of unloaded runners.
func (l *loader) Unload(ctx context.Context, unload UnloadRequest) int {
	if !l.lock(ctx) {
		return 0
	}
	defer l.unlock()

	return len(l.runners) - func() int {
		if unload.All {
			l.runnerConfigs = make(map[runnerKey]inference.BackendConfiguration)
			return l.evict(false)
		} else {
			for _, model := range unload.Models {
				modelID := l.modelManager.ResolveID(model)
				// Delete all runner configs for this model (including with different draft models)
				for key := range l.runnerConfigs {
					if key.backend == unload.Backend && key.modelID == modelID {
						delete(l.runnerConfigs, key)
					}
				}
				// Evict all mode types. We should consider
				// accepting a mode parameter in unload requests.
				l.evictRunner(unload.Backend, modelID, inference.BackendModeCompletion)
				l.evictRunner(unload.Backend, modelID, inference.BackendModeEmbedding)
				l.evictRunner(unload.Backend, modelID, inference.BackendModeReranking)
				l.evictRunner(unload.Backend, modelID, inference.BackendModeImageGeneration)
			}
			return len(l.runners)
		}
	}()
}

// stopAndDrainTimer stops and drains a timer without knowing if it was running.
func stopAndDrainTimer(timer *time.Timer) {
	timer.Stop()
	select {
	case <-timer.C:
	default:
	}
}

// idleCheckDuration computes the duration until the next idle runner eviction
// should occur. The caller must hold the loader lock. If no runners are unused,
// then -1 seconds is returned. If any unused runners are already expired, then
// 0 seconds is returned. Otherwise a time in the future at which eviction
// should occur is returned.
func (l *loader) idleCheckDuration() time.Duration {
	soonest := time.Duration(-1) * time.Second
	hasCandidate := false

	for r, runnerInfo := range l.runners {
		select {
		case <-l.slots[runnerInfo.slot].done:
			// Check immediately if a runner is defunct
			return 0
		default:
		}
		if l.references[runnerInfo.slot] == 0 {
			timeout := l.runnerIdleTimeoutFor(r)
			// Skip runners that should never be evicted
			if timeout < 0 {
				continue
			}
			remaining := timeout - time.Since(l.timestamps[runnerInfo.slot])
			if remaining < 0 {
				return 0
			}
			remaining += 100 * time.Millisecond
			if !hasCandidate || remaining < soonest {
				soonest = remaining
				hasCandidate = true
			}
		}
	}

	return soonest
}

// run is the run loop for the loader. It drives idle runner eviction. By the
// time run returns, all runners will have been evicted.
func (l *loader) run(ctx context.Context) {
	// Signal that loads are enabled. There's no need to broadcast here because
	// no loaders will wait if they see that loads are disabled.
	if !l.lock(ctx) {
		return
	}
	l.loadsEnabled = true
	l.unlock()

	// Defer disablement of loads and wait for complete eviction.
	defer func() {
		poll := make(chan struct{}, 1)
		poll <- struct{}{} // Trigger an initial polling in case all are unused.
		l.lock(context.Background())
		l.loadsEnabled = false
		l.broadcast()
		l.waiters[poll] = true
		l.unlock()
		for range poll {
			l.lock(context.Background())
			if l.evict(false) == 0 {
				delete(l.waiters, poll)
				l.unlock()
				break
			}
			l.unlock()
		}
	}()

	// Create a timer that we'll use to drive idle eviction. Ensure that it's
	// stopped by the time we exit.
	idleTimer := time.NewTimer(0)
	if !idleTimer.Stop() {
		<-idleTimer.C
	}
	defer idleTimer.Stop()

	// Evict idle runners.
	for {
		select {
		case <-ctx.Done():
			return
		case <-idleTimer.C:
			// Perform eviction.
			if l.lock(ctx) {
				l.evict(true)
				if nextCheck := l.idleCheckDuration(); nextCheck >= 0 {
					idleTimer.Reset(nextCheck)
				}
				l.unlock()
			}
		case <-l.idleCheck:
			// Compute the next idle check time.
			if l.lock(ctx) {
				stopAndDrainTimer(idleTimer)
				if nextCheck := l.idleCheckDuration(); nextCheck >= 0 {
					idleTimer.Reset(nextCheck)
				}
				l.unlock()
			}
		}
	}
}

// usedSlots returns the number of slots that are either occupied by a
// registered runner or reserved for a runner being loaded.
func (l *loader) usedSlots() int {
	return len(l.runners) + len(l.loading)
}

// isSlotLoading reports whether the given slot is reserved for a runner
// that is currently being initialized.
func (l *loader) isSlotLoading(slot int) bool {
	_, ok := l.loading[slot]
	return ok
}

// isModelLoading reports whether a runner for the given model is currently
// being initialized by another goroutine.
func (l *loader) isModelLoading(backendName, modelID, draftModelID string, mode inference.BackendMode) bool {
	for _, info := range l.loading {
		if info.backendName == backendName && info.modelID == modelID && info.draftModelID == draftModelID && info.mode == mode {
			return true
		}
	}
	return false
}

// load allocates a runner using the specified backend and modelID. If allocated,
// it should be released by the caller using the release mechanism (once the
// runner is no longer needed).
func (l *loader) load(ctx context.Context, backendName, modelID, modelRef string, mode inference.BackendMode) (*runner, error) {
	// Grab the backend. The backends map is immutable after construction,
	// so it is safe to read without holding the lock.
	backend, ok := l.backends[backendName]
	if !ok {
		return nil, ErrBackendNotFound
	}

	l.log.Info("Loading backend runner", "backend", backendName, "model", modelID, "mode", mode)

	if !l.lock(ctx) {
		return nil, context.Canceled
	}
	// Note: the lock is managed explicitly throughout this function rather
	// than via defer, because it is released during long-running operations
	// (run + wait) and re-acquired afterwards.

	// Get runner configuration if available (must be done under lock since
	// runnerConfigs can be modified concurrently by setRunnerConfig).
	var runnerConfig *inference.BackendConfiguration
	draftModelID := ""
	if rc, ok := l.runnerConfigs[makeConfigKey(backendName, modelID, mode)]; ok {
		runnerConfig = &rc
		if runnerConfig.Speculative != nil && runnerConfig.Speculative.DraftModel != "" {
			draftModelID = l.modelManager.ResolveID(runnerConfig.Speculative.DraftModel)
		}
	} else if (mode == inference.BackendModeReranking) || (mode == inference.BackendModeImageGeneration) {
		// For reranking or image-generation mode, fallback to completion config if specific config is not found.
		if rc, ok := l.runnerConfigs[makeConfigKey(backendName, modelID, inference.BackendModeCompletion)]; ok {
			runnerConfig = &rc
			if runnerConfig.Speculative != nil && runnerConfig.Speculative.DraftModel != "" {
				draftModelID = l.modelManager.ResolveID(runnerConfig.Speculative.DraftModel)
			}
		}
	}

	// If no explicit config exists, create a default one with the model's context size
	// so that the OpenAI recorder can report the actual configuration being used.
	if runnerConfig == nil {
		defaultConfig := inference.BackendConfiguration{}
		if l.modelManager != nil {
			if bundle, err := l.modelManager.GetBundle(modelID); err != nil {
				l.log.Warn("Failed to get bundle for model to determine default context size", "model", modelID, "error", err)
			} else if runtimeConfig := bundle.RuntimeConfig(); runtimeConfig != nil {
				if ctxSize := runtimeConfig.GetContextSize(); ctxSize != nil {
					defaultConfig.ContextSize = ctxSize
				}
			}
		}
		runnerConfig = &defaultConfig
	}

	// Create a polling channel that we can use to detect state changes and
	// ensure that it's deregistered by the time we return.
	poll := make(chan struct{}, 1)
	l.waiters[poll] = true

	// cleanupAndReturn is a helper that cleans up the waiter registration,
	// releases the lock, and returns. All exit paths must go through this
	// to avoid leaking the poll channel or double-unlocking.
	cleanupAndReturn := func(r *runner, err error) (*runner, error) {
		delete(l.waiters, poll)
		l.unlock()
		return r, err
	}

	// Loop until we can satisfy the request or an error occurs.
	// These are declared outside the loop to avoid goto-over-declaration errors.
	var existing runnerInfo
	var existingOK bool
	for {
		slot := -1

		// If loads are disabled, then there's nothing we can do.
		if !l.loadsEnabled {
			return cleanupAndReturn(nil, errLoadsDisabled)
		}

		// See if another goroutine is already loading this runner.
		// If so, wait for it to finish rather than starting a duplicate load.
		if l.isModelLoading(backendName, modelID, draftModelID, mode) {
			goto WaitForChange
		}

		// See if we can satisfy the request with an existing runner.
		existing, existingOK = l.runners[makeRunnerKey(backendName, modelID, draftModelID, mode)]
		if existingOK {
			select {
			case <-l.slots[existing.slot].done:
				l.log.Warn("Runner is defunct, waiting for eviction", "backend", backendName, "model", existing.modelRef)
				if l.references[existing.slot] == 0 {
					l.evictRunner(backendName, modelID, mode)
					// Continue the loop to retry loading after evicting the defunct runner
					continue
				} else {
					goto WaitForChange
				}
			default:
				l.references[existing.slot]++
				l.timestamps[existing.slot] = time.Time{}
				return cleanupAndReturn(l.slots[existing.slot], nil)
			}
		}

		// If all slots are full (including loading reservations), try evicting unused runners.
		if l.usedSlots() >= len(l.slots) {
			l.log.Info("Evicting to make room", "runners", len(l.runners), "loading", len(l.loading), "slots", len(l.slots))
			runnerCountAtLoopStart := len(l.runners)
			remainingRunners := l.evict(false)
			// Restart the loop if eviction happened
			if remainingRunners < runnerCountAtLoopStart {
				continue
			}
		}

		// If there's a free slot, then find one that is not reserved for loading.
		if l.usedSlots() < len(l.slots) {
			for s, runner := range l.slots {
				if runner == nil && !l.isSlotLoading(s) {
					slot = s
					break
				}
			}
		}

		if slot < 0 {
			l.log.Debug("Cannot load model yet", "runners", len(l.runners), "loading", len(l.loading), "slots", len(l.slots))
		}

		// If we've identified a slot, then we're ready to start a runner.
		if slot >= 0 {
			// Reserve the slot and release the lock for the long-running
			// operations (run + wait). This allows other goroutines to
			// proceed with loading different models, releasing runners, etc.
			l.loading[slot] = loadingInfo{
				backendName:  backendName,
				modelID:      modelID,
				draftModelID: draftModelID,
				modelRef:     modelRef,
				mode:         mode,
			}
			l.unlock()

			newRunner, err := run(l.log, backend, modelID, modelRef, mode, slot, runnerConfig, l.openAIRecorder)
			if err != nil {
				l.log.Warn("Unable to start backend runner", "backend", backendName, "model", modelID, "mode", mode, "error", err)
				l.lock(context.Background())
				delete(l.loading, slot)
				l.broadcast()
				return cleanupAndReturn(nil, fmt.Errorf("unable to start runner: %w", err))
			}

			if err := newRunner.wait(ctx); err != nil {
				newRunner.terminate()
				l.log.Warn("Backend runner initialization failed", "backend", backendName, "model", modelID, "mode", mode, "error", err)
				l.lock(context.Background())
				delete(l.loading, slot)
				l.broadcast()
				return cleanupAndReturn(nil, fmt.Errorf("error waiting for runner to be ready: %w", err))
			}

			// Re-acquire lock and register the runner.
			l.lock(context.Background())
			delete(l.loading, slot)

			// Perform registration and return the runner.
			l.runners[makeRunnerKey(backendName, modelID, draftModelID, mode)] = runnerInfo{slot, modelRef}
			l.slots[slot] = newRunner
			l.references[slot] = 1
			l.broadcast()
			return cleanupAndReturn(newRunner, nil)
		}

		// Wait for something to change. Note that we always re-lock with
		// context.Background() because we need to ensure we hold the lock by
		// the time we return.
	WaitForChange:
		l.unlock()
		select {
		case <-ctx.Done():
			l.lock(context.Background())
			return cleanupAndReturn(nil, context.Canceled)
		case <-poll:
			l.lock(context.Background())
		}
	}
}

// release releases a runner, which internally decrements its reference count.
func (l *loader) release(runner *runner) {
	// Acquire the loader lock and defer its release.
	l.lock(context.Background())
	defer l.unlock()

	// Find the runner's slot by iterating through runners
	var slotInfo runnerInfo
	for key, info := range l.runners {
		if key.backend == runner.backend.Name() && key.modelID == runner.model && key.mode == runner.mode {
			slotInfo = info
			break
		}
	}

	// Decrement the runner's reference count.
	l.references[slotInfo.slot]--

	// If the runner's reference count is now zero, then check if it is still
	// active, and record now as its idle start time and signal the idle
	// checker.
	if l.references[slotInfo.slot] == 0 {
		select {
		case <-runner.done:
			l.evictRunner(runner.backend.Name(), runner.model, runner.mode)
		default:
			l.timestamps[slotInfo.slot] = time.Now()
			select {
			case l.idleCheck <- struct{}{}:
			default:
			}
		}
	}

	// Signal waiters.
	l.broadcast()
}

func (l *loader) setRunnerConfig(ctx context.Context, backendName, modelID string, mode inference.BackendMode, runnerConfig inference.BackendConfiguration) error {
	l.lock(ctx)
	defer l.unlock()

	// Configuration key should NOT include draftModelID since that's part of the config itself
	configKey := makeConfigKey(backendName, modelID, mode)

	// If the configuration hasn't changed, then just return.
	if existingConfig, ok := l.runnerConfigs[configKey]; ok && reflect.DeepEqual(runnerConfig, existingConfig) {
		l.log.Info("Runner configuration unchanged", "backend", backendName, "model", modelID)
		return nil
	}

	// Determine the draftModelID from the config to find any existing runner
	draftModelID := ""
	if runnerConfig.Speculative != nil && runnerConfig.Speculative.DraftModel != "" {
		draftModelID = l.modelManager.ResolveID(runnerConfig.Speculative.DraftModel)
	}
	rKey := makeRunnerKey(backendName, modelID, draftModelID, mode)

	// If there's an active runner whose configuration we want to override, then
	// try evicting it (because it may not be in use).
	if _, ok := l.runners[rKey]; ok {
		l.evictRunner(backendName, modelID, mode)
	}

	// If there's still then active runner, then we can't (or at least
	// shouldn't) change the configuration.
	if _, ok := l.runners[rKey]; ok {
		return errRunnerAlreadyActive
	}

	l.log.Info("Configuring runner", "backend", backendName, "model", modelID)
	l.runnerConfigs[configKey] = runnerConfig
	return nil
}

// getAllRunnerConfigs retrieves all runner configurations.
func (l *loader) getAllRunnerConfigs(ctx context.Context) []ModelConfigEntry {
	if !l.lock(ctx) {
		return nil
	}
	defer l.unlock()

	entries := make([]ModelConfigEntry, 0, len(l.runnerConfigs))
	for key, config := range l.runnerConfigs {
		model, err := l.modelManager.GetLocal(key.modelID)
		if err == nil {
			modelName := ""
			if len(model.Tags()) > 0 {
				modelName = model.Tags()[0]
			}
			entries = append(entries, ModelConfigEntry{
				Backend: key.backend,
				Model:   modelName,
				ModelID: key.modelID,
				Mode:    key.mode,
				Config:  config,
			})
		}
	}
	return entries
}
