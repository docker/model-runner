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
	"github.com/docker/model-runner/pkg/inference/memory"
	"github.com/docker/model-runner/pkg/inference/models"
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
	// errModelTooBig indicates that the model is too big to ever load into the
	// available system memory.
	errModelTooBig = errors.New("model too big")
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
	// mode is the operation mode associated with the runner.
	mode inference.BackendMode
}

// runnerInfo holds information about a runner including its slot and the original model reference used to load it.
type runnerInfo struct {
	// slot is the slot index where the runner is stored.
	slot int
	// modelRef is the original model reference (tag) used to load the runner.
	modelRef string
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
	// totalMemory is the total system memory allocated to the loader.
	totalMemory inference.RequiredMemory
	// idleCheck is used to signal the run loop when timestamps have updated.
	idleCheck chan struct{}
	// guard is a sempahore controlling access to all subsequent fields. It is
	// buffered (with size 1) and contains a single element that must be held in
	// order to operate on those fields. We use a channel (instead of a
	// sync.Mutex) to enable polling.
	guard chan struct{}
	// loadsEnabled signals that loads are currently enabled.
	loadsEnabled bool
	// availableMemory is the available portion of the loader's total memory.
	availableMemory inference.RequiredMemory
	// waiters is the set of signal channels associated with waiting loaders. We
	// use a set of signaling channels (instead of a sync.Cond) to enable
	// polling. Each signaling channel should be buffered (with size 1).
	waiters map[chan<- struct{}]bool
	// runners maps runner keys to their slot index.
	runners map[runnerKey]runnerInfo
	// slots maps slot indices to associated runners. A slot is considered free
	// if the runner value in it is nil.
	slots []*runner
	// references maps slot indices to reference counts.
	references []uint
	// allocations maps slot indices to memory allocation sizes.
	allocations []inference.RequiredMemory
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
	sysMemInfo memory.SystemMemoryInfo,
) *loader {
	// Compute the number of runner slots to allocate. Because of RAM and VRAM
	// limitations, it's unlikely that we'll ever be able to fully populate
	// these slots, so for now we just choose a reasonable value. We may need to
	// tune this heuristic for systems with enormous amounts of VRAM.
	nSlots := min(runtime.NumCPU(), maximumRunnerSlots)

	// Check if we have a special environment.
	isGPUEnabledCloudEnvironment := environment.Get() == environment.EnvironmentCloud &&
		os.Getenv("NVIDIA_VISIBLE_DEVICES") != ""

	// Compute the idle runner timeout.
	//
	// HACK: On GPU-enabled cloud engines, we'll bump this to 8 hours. We can
	// remove this once we have configurable TTLs.
	runnerIdleTimeout := defaultRunnerIdleTimeout
	if isGPUEnabledCloudEnvironment {
		runnerIdleTimeout = 8 * time.Hour
	}

	// Compute the amount of available memory.
	totalMemory := sysMemInfo.GetTotalMemory()

	// Create the loader.
	l := &loader{
		log:               log,
		backends:          backends,
		modelManager:      modelManager,
		runnerIdleTimeout: runnerIdleTimeout,
		totalMemory:       totalMemory,
		idleCheck:         make(chan struct{}, 1),
		guard:             make(chan struct{}, 1),
		availableMemory:   totalMemory,
		waiters:           make(map[chan<- struct{}]bool),
		runners:           make(map[runnerKey]runnerInfo, nSlots),
		slots:             make([]*runner, nSlots),
		references:        make([]uint, nSlots),
		allocations:       make([]inference.RequiredMemory, nSlots),
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

// evict evicts all unused runners from the loader. If idleOnly is true, then
// only those unused, but functioning, runners which are considered "idle" (based
// on usage timestamp) are evicted. Defunct (e.g. crashed) runners will be evicted
// regardless of whether they are considered "idle". The caller must hold the loader
// lock. It returns the number of remaining runners.
func (l *loader) evict(idleOnly bool) int {
	now := time.Now()
	for r, runnerInfo := range l.runners {
		unused := l.references[runnerInfo.slot] == 0
		idle := unused && now.Sub(l.timestamps[runnerInfo.slot]) > l.runnerIdleTimeout
		defunct := false
		select {
		case <-l.slots[runnerInfo.slot].done:
			defunct = true
		default:
		}
		if unused && (!idleOnly || idle || defunct) {
			l.log.Infof("Evicting %s backend runner with model %s (%s) in %s mode",
				r.backend, r.modelID, runnerInfo.modelRef, r.mode,
			)
			l.slots[runnerInfo.slot].terminate()
			l.slots[runnerInfo.slot] = nil
			l.availableMemory.RAM += l.allocations[runnerInfo.slot].RAM
			l.availableMemory.VRAM += l.allocations[runnerInfo.slot].VRAM
			l.allocations[runnerInfo.slot] = inference.RequiredMemory{RAM: 0, VRAM: 0}
			l.timestamps[runnerInfo.slot] = time.Time{}
			delete(l.runners, r)
		}
	}
	return len(l.runners)
}

// evictRunner evicts a specific runner. The caller must hold the loader lock.
// It returns the number of remaining runners.
func (l *loader) evictRunner(backend, model string, mode inference.BackendMode) int {
	allBackends := backend == ""
	for r, runnerInfo := range l.runners {
		unused := l.references[runnerInfo.slot] == 0
		if unused && (allBackends || r.backend == backend) && r.modelID == model && r.mode == mode {
			l.log.Infof("Evicting %s backend runner with model %s (%s) in %s mode",
				r.backend, r.modelID, runnerInfo.modelRef, r.mode,
			)
			l.slots[runnerInfo.slot].terminate()
			l.slots[runnerInfo.slot] = nil
			l.availableMemory.RAM += l.allocations[runnerInfo.slot].RAM
			l.availableMemory.VRAM += l.allocations[runnerInfo.slot].VRAM
			l.allocations[runnerInfo.slot] = inference.RequiredMemory{RAM: 0, VRAM: 0}
			l.timestamps[runnerInfo.slot] = time.Time{}
			delete(l.runners, r)
		}
	}
	return len(l.runners)
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
				modelID := l.modelManager.ResolveModelID(model)
				delete(l.runnerConfigs, runnerKey{unload.Backend, modelID, inference.BackendModeCompletion})
				delete(l.runnerConfigs, runnerKey{unload.Backend, modelID, inference.BackendModeEmbedding})
				// Evict both, completion and embedding models. We should consider
				// accepting a mode parameter in unload requests.
				l.evictRunner(unload.Backend, modelID, inference.BackendModeCompletion)
				l.evictRunner(unload.Backend, modelID, inference.BackendModeEmbedding)
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
	// Compute the oldest usage time for any idle runner.
	var oldest time.Time
	for _, runnerInfo := range l.runners {
		select {
		case <-l.slots[runnerInfo.slot].done:
			// Check immediately if a runner is defunct
			return 0
		default:
		}
		if l.references[runnerInfo.slot] == 0 {
			timestamp := l.timestamps[runnerInfo.slot]
			if oldest.IsZero() || timestamp.Before(oldest) {
				oldest = timestamp
			}
		}
	}

	// If there are no unused runners, then don't schedule a check.
	if oldest.IsZero() {
		return -1 * time.Second
	}

	// Compute the remaining duration. If negative, check immediately, otherwise
	// wait until 100 milliseconds after expiration time (to avoid checking
	// right on the expiration boundary).
	if remaining := l.runnerIdleTimeout - time.Since(oldest); remaining < 0 {
		return 0
	} else {
		return remaining + 100*time.Millisecond
	}
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

// load allocates a runner using the specified backend and modelID. If allocated,
// it should be released by the caller using the release mechanism (once the
// runner is no longer needed).
func (l *loader) load(ctx context.Context, backendName, modelID, modelRef string, mode inference.BackendMode) (*runner, error) {
	// Grab the backend.
	backend, ok := l.backends[backendName]
	if !ok {
		return nil, ErrBackendNotFound
	}

	// Estimate the amount of memory that will be used by the model and check
	// that we're even capable of loading it.
	var runnerConfig *inference.BackendConfiguration
	if rc, ok := l.runnerConfigs[runnerKey{backendName, modelID, mode}]; ok {
		runnerConfig = &rc
	}
	memory, err := backend.GetRequiredMemoryForModel(ctx, modelID, runnerConfig)
	var parseErr *inference.ErrGGUFParse
	if errors.As(err, &parseErr) {
		// TODO(p1-0tr): For now override memory checks in case model can't be parsed
		// e.g. model is too new for gguf-parser-go to know. We should provide a cleaner
		// way to bypass these checks.
		l.log.Warnf("Could not parse model(%s), memory checks will be ignored for it. Error: %s", modelID, parseErr)
		memory = inference.RequiredMemory{
			RAM:  0,
			VRAM: 0,
		}
	} else if err != nil {
		return nil, err
	}
	l.log.Infof("Loading %s, which will require %d MB RAM and %d MB VRAM on a system with %d MB RAM and %d MB VRAM", modelID, memory.RAM/1024/1024, memory.VRAM/1024/1024, l.totalMemory.RAM/1024/1024, l.totalMemory.VRAM/1024/1024)
	if l.totalMemory.RAM == 1 {
		l.log.Warnf("RAM size unknown. Assume model will fit, but only one.")
		memory.RAM = 1
	}
	if l.totalMemory.VRAM == 1 {
		l.log.Warnf("VRAM size unknown. Assume model will fit, but only one.")
		memory.VRAM = 1
	}
	// Validate if model could fit.
	// On Windows, llamacpp can use up to half of system RAM as shared GPU memory
	// if it runs out of dedicated VRAM.
	totalVRAM := l.totalMemory.VRAM
	if runtime.GOOS == "windows" {
		totalVRAM += l.totalMemory.RAM / 2
	}
	if memory.RAM > l.totalMemory.RAM || memory.VRAM > totalVRAM {
		return nil, errModelTooBig
	}

	// Acquire the loader lock and defer its release.
	if !l.lock(ctx) {
		return nil, context.Canceled
	}
	defer l.unlock()

	// Create a polling channel that we can use to detect state changes and
	// ensure that it's deregistered by the time we return.
	poll := make(chan struct{}, 1)
	l.waiters[poll] = true
	defer func() {
		delete(l.waiters, poll)
	}()

	// Loop until we can satisfy the request or an error occurs.
	for {
		slot := -1
		availableVRAM := l.availableMemory.VRAM

		// If loads are disabled, then there's nothing we can do.
		if !l.loadsEnabled {
			return nil, errLoadsDisabled
		}

		// See if we can satisfy the request with an existing runner.
		existing, ok := l.runners[runnerKey{backendName, modelID, mode}]
		if ok {
			select {
			case <-l.slots[existing.slot].done:
				l.log.Warnf("%s runner for %s is defunct. Waiting for it to be evicted.", backendName, existing.modelRef)
				if l.references[existing.slot] == 0 {
					l.evictRunner(backendName, modelID, mode)
				} else {
					goto WaitForChange
				}
			default:
				l.references[existing.slot] += 1
				l.timestamps[existing.slot] = time.Time{}
				return l.slots[existing.slot], nil
			}
		}

		if runtime.GOOS == "windows" {
			// On Windows, we can use up to half of the total system RAM as shared GPU memory,
			// limited by the currently available RAM.
			sharedRAM := l.totalMemory.RAM / 2
			if l.availableMemory.RAM < sharedRAM {
				sharedRAM = l.availableMemory.RAM
			}
			availableVRAM += sharedRAM
		}

		// If there's not sufficient memory or all slots are full, then try
		// evicting unused runners.
		if memory.RAM > l.availableMemory.RAM || memory.VRAM > availableVRAM || len(l.runners) == len(l.slots) {
			l.evict(false)
		}

		// If there's sufficient memory and a free slot, then find the slot.
		if memory.RAM <= l.availableMemory.RAM && memory.VRAM <= availableVRAM && len(l.runners) < len(l.slots) {
			for s, runner := range l.slots {
				if runner == nil {
					slot = s
					break
				}
			}
		}

		// If we've identified a slot, then we're ready to start a runner.
		if slot >= 0 {
			var runnerConfig *inference.BackendConfiguration
			if rc, ok := l.runnerConfigs[runnerKey{backendName, modelID, mode}]; ok {
				runnerConfig = &rc
			}
			// Create the runner.
			l.log.Infof("Loading %s backend runner with model %s in %s mode", backendName, modelID, mode)
			runner, err := run(l.log, backend, modelID, mode, slot, runnerConfig, l.openAIRecorder)
			if err != nil {
				l.log.Warnf("Unable to start %s backend runner with model %s in %s mode: %v",
					backendName, modelID, mode, err,
				)
				return nil, fmt.Errorf("unable to start runner: %w", err)
			}

			// Wait for the runner to be ready. In theory it's a little
			// inefficient to block all other loaders (including those that
			// might not want this runner), but in reality they would probably
			// be blocked by the underlying loading anyway (in terms of disk and
			// GPU performance). We have to retain a lock here though to enforce
			// deduplication of runners and keep slot / memory reservations.
			if err := runner.wait(ctx); err != nil {
				runner.terminate()
				l.log.Warnf("Initialization for %s backend runner with model %s in %s mode failed: %v",
					backendName, modelID, mode, err,
				)
				return nil, fmt.Errorf("error waiting for runner to be ready: %w", err)
			}

			// Perform registration and return the runner.
			l.availableMemory.RAM -= memory.RAM
			l.availableMemory.VRAM -= memory.VRAM
			l.runners[runnerKey{backendName, modelID, mode}] = runnerInfo{slot, modelRef}
			l.slots[slot] = runner
			l.references[slot] = 1
			l.allocations[slot].RAM = memory.RAM
			l.allocations[slot].VRAM = memory.VRAM
			return runner, nil
		}

		// Wait for something to change. Note that we always re-lock with
		// context.Background() because we need to ensure we hold the lock by
		// the time we return.
	WaitForChange:
		l.unlock()
		select {
		case <-ctx.Done():
			l.lock(context.Background())
			return nil, context.Canceled
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

	// Determine the runner's slot.
	slot := l.runners[runnerKey{runner.backend.Name(), runner.model, runner.mode}]

	// Decrement the runner's reference count.
	l.references[slot.slot] -= 1

	// If the runner's reference count is now zero, then check if it is still
	// active, and record now as its idle start time and signal the idle
	// checker.
	if l.references[slot.slot] == 0 {
		select {
		case <-runner.done:
			l.evictRunner(runner.backend.Name(), runner.model, runner.mode)
		default:
			l.timestamps[slot.slot] = time.Now()
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

	runnerId := runnerKey{backendName, modelID, mode}

	// If the configuration hasn't changed, then just return.
	if existingConfig, ok := l.runnerConfigs[runnerId]; ok && reflect.DeepEqual(runnerConfig, existingConfig) {
		l.log.Infof("Configuration for %s runner for modelID %s unchanged", backendName, modelID)
		return nil
	}

	// If there's an active runner whose configuration we want to override, then
	// try evicting it (because it may not be in use).
	if _, ok := l.runners[runnerId]; ok {
		l.evictRunner(backendName, modelID, mode)
	}

	// If there's still then active runner, then we can't (or at least
	// shouldn't) change the configuration.
	if _, ok := l.runners[runnerId]; ok {
		return errRunnerAlreadyActive
	}

	l.log.Infof("Configuring %s runner for %s", backendName, modelID)
	l.runnerConfigs[runnerId] = runnerConfig
	return nil
}
