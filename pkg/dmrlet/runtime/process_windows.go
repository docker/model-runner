//go:build windows

package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// ProcessRuntime runs inference workloads as native OS processes (for Windows).
type ProcessRuntime struct {
	log     *logrus.Entry
	procs   map[string]*managedProcess
	procsMu sync.Mutex
}

type managedProcess struct {
	cmd     *exec.Cmd
	spec    ContainerSpec
	started time.Time
}

// NewProcessRuntime creates a new process-based runtime.
func NewProcessRuntime(log *logrus.Entry) *ProcessRuntime {
	return &ProcessRuntime{
		log:   log,
		procs: make(map[string]*managedProcess),
	}
}

// Run starts a process based on the container spec.
func (r *ProcessRuntime) Run(ctx context.Context, spec ContainerSpec) error {
	r.procsMu.Lock()
	defer r.procsMu.Unlock()

	if _, exists := r.procs[spec.ID]; exists {
		return fmt.Errorf("process %s already running", spec.ID)
	}

	if len(spec.Command) == 0 {
		return fmt.Errorf("no command specified for %s", spec.ID)
	}

	// Translate container mount paths to host paths in command args
	args := translateMountPaths(spec.Command, spec.Mounts)

	// Try com.docker.llama-server first, fall back to llama-server
	args[0] = resolveExecutable(args[0])

	// Validate that the executable path is safe (doesn't contain path separators)
	executable := filepath.Base(args[0]) // Only use the base name to prevent path traversal
	if executable == "" || executable == "." || executable == ".." {
		return fmt.Errorf("invalid executable path: %s", args[0])
	}

	// Validate arguments to prevent command injection
	for i, arg := range args[1:] {
		// Check for potential command injection patterns
		if strings.Contains(arg, ";") || strings.Contains(arg, "&") ||
			strings.Contains(arg, "|") || strings.Contains(arg, "`") ||
			strings.Contains(arg, "$(") || strings.Contains(arg, "\n") {
			return fmt.Errorf("unsafe argument detected at position %d: %s", i+1, arg)
		}
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = append(os.Environ(), spec.Env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if spec.WorkingDir != "" {
		cmd.Dir = spec.WorkingDir
	}

	// On Windows, we don't use Setpgid as it's not available
	// cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // Not available on Windows

	r.log.Infof("Starting process %s: %v", spec.ID, args)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting process %s: %w", spec.ID, err)
	}

	r.procs[spec.ID] = &managedProcess{
		cmd:     cmd,
		spec:    spec,
		started: time.Now(),
	}

	// Reap the process in the background
	go func() {
		if err := cmd.Wait(); err != nil {
			r.log.Warnf("Process %s exited: %v", spec.ID, err)
		} else {
			r.log.Infof("Process %s exited normally", spec.ID)
		}
		r.procsMu.Lock()
		delete(r.procs, spec.ID)
		r.procsMu.Unlock()
	}()

	r.log.Infof("Process %s started (PID %d)", spec.ID, cmd.Process.Pid)
	return nil
}

// Stop stops a running process.
func (r *ProcessRuntime) Stop(ctx context.Context, id string) error {
	r.procsMu.Lock()
	proc, exists := r.procs[id]
	if !exists {
		r.procsMu.Unlock()
		return fmt.Errorf("process %s not found", id)
	}
	r.procsMu.Unlock()

	// On Windows, we can't use process groups like Unix systems
	// Just signal the main process
	proc.cmd.Process.Signal(nil) // This doesn't actually send a signal on Windows, just check if process is alive

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		proc.cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		r.log.Warnf("Process %s did not exit after timeout, sending Kill", id)
		proc.cmd.Process.Kill()
	}

	r.procsMu.Lock()
	delete(r.procs, id)
	r.procsMu.Unlock()

	r.log.Infof("Process %s stopped", id)
	return nil
}

// List returns all managed processes.
func (r *ProcessRuntime) List(ctx context.Context) ([]ContainerInfo, error) {
	r.procsMu.Lock()
	defer r.procsMu.Unlock()

	var result []ContainerInfo
	for id, proc := range r.procs {
		result = append(result, ContainerInfo{
			ID:      id,
			Image:   proc.spec.Image,
			Status:  "running",
			Labels:  map[string]string{"dmrlet.managed": "true"},
			Created: proc.started,
		})
	}
	return result, nil
}

// Exists checks if a process exists.
func (r *ProcessRuntime) Exists(ctx context.Context, id string) (bool, error) {
	r.procsMu.Lock()
	defer r.procsMu.Unlock()
	_, exists := r.procs[id]
	return exists, nil
}

// Close stops all managed processes.
func (r *ProcessRuntime) Close() error {
	r.procsMu.Lock()
	ids := make([]string, 0, len(r.procs))
	for id := range r.procs {
		ids = append(ids, id)
	}
	r.procsMu.Unlock()

	for _, id := range ids {
		if err := r.Stop(context.Background(), id); err != nil {
			r.log.Warnf("Failed to stop process %s: %v", id, err)
		}
	}
	return nil
}

// resolveExecutable checks for a "com.docker." prefixed variant of the executable
// (e.g. com.docker.llama-server) and returns it if found, otherwise returns the
// original name.
func resolveExecutable(name string) string {
	dockerName := "com.docker." + name
	if path, err := exec.LookPath(dockerName); err == nil {
		return path
	}
	return name
}

// translateMountPaths rewrites container-internal paths in command arguments
// to host paths using the mount mappings. For example, if there's a mount
// from /host/models → /model, then "/model/foo.gguf" becomes "/host/models/foo.gguf".
func translateMountPaths(command []string, mounts []Mount) []string {
	if len(mounts) == 0 {
		return command
	}

	result := make([]string, len(command))
	for i, arg := range command {
		result[i] = arg
		for _, m := range mounts {
			dest := m.Destination
			if arg == dest || strings.HasPrefix(arg, dest+"/") {
				rel, _ := filepath.Rel(dest, arg)
				result[i] = filepath.Join(m.Source, rel)
				break
			}
		}
	}
	return result
}

// Compile-time check.
var _ Runner = (*ProcessRuntime)(nil)
