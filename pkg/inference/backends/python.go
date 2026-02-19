package backends

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

// ErrPythonNotFound is returned when python3 cannot be found.
var ErrPythonNotFound = errors.New("python3 not found in PATH")

// FindPythonPath returns the path to a python3 binary.
// If customPath is non-empty, it is returned directly.
// If envDir is non-empty and envDir/bin/python3 exists, that is returned.
// Otherwise, python3 is looked up in PATH. Returns ErrPythonNotFound if none found.
func FindPythonPath(customPath, envDir string) (string, error) {
	if customPath != "" {
		return customPath, nil
	}
	if envDir != "" {
		venvPython := filepath.Join(envDir, "bin", "python3")
		if _, err := os.Stat(venvPython); err == nil {
			return venvPython, nil
		}
	}
	systemPython, err := exec.LookPath("python3")
	if err != nil {
		return "", ErrPythonNotFound
	}
	return systemPython, nil
}

// NewPythonCmd creates an exec.Cmd that runs python3 with the given arguments.
// If pythonPath is empty, "python3" is used.
func NewPythonCmd(pythonPath string, args ...string) *exec.Cmd {
	binary := "python3"
	if pythonPath != "" {
		binary = pythonPath
	}
	return exec.Command(binary, args...)
}
