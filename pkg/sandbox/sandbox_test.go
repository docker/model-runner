package sandbox

import (
	"runtime"
	"testing"
)

// TestSandbox performs basic sandbox testing.
func TestSandbox(t *testing.T) {
	var sandbox Sandbox
	var err error
	if runtime.GOOS == "windows" {
		sandbox, err = Create(t.Context(), ConfigurationLlamaCpp, nil, "/some/path/bin", "go", "version")
	} else {
		sandbox, err = Create(t.Context(), ConfigurationLlamaCpp, nil, "/some/path/bin", "date")
	}
	if err != nil {
		t.Fatal("unable to create sandboxed process:", err)
	}
	err = sandbox.Command().Wait()
	if err != nil {
		t.Error("unable to wait for process completion:", err)
	}
	err = sandbox.Close()
	if err != nil {
		t.Error("sandbox closure failed:", err)
	}
}
