//go:build !windows

package iniconfig_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/docker/model-runner/cmd/cli/iniconfig"
)

func TestSet_PreservesModeUnderUmask(t *testing.T) {
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)

	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("[core]\n\tbare = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := iniconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Set("core.filemode", "true"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("expected mode 0644, got %v", info.Mode().Perm())
	}
}
