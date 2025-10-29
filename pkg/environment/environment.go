package environment

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// Environment encodes the operating environment for the model runner.
type Environment uint8

const (
	// EnvironmentUnknown represents an unknown environment in which basic,
	// non-specialized defaults should be used.
	EnvironmentUnknown Environment = iota
	// EnvironmentDesktop represents a Docker Desktop environment.
	EnvironmentDesktop
	// EnvironmentMoby represents a Moby engine environment, if installed via
	// the model CLI.
	EnvironmentMoby
	// EnvironmentCloud represents a Docker Cloud environment, if installed via
	// the model CLI.
	EnvironmentCloud
)

// environment is the cached environment.
var environment Environment

// environmentOnce guards initialization of environment.
var environmentOnce sync.Once

// isDockerBackend checks if an executable path is com.docker.backend.
func isDockerBackend(path string) bool {
	leaf := filepath.Base(path)
	if runtime.GOOS == "windows" {
		return leaf == "com.docker.backend.exe"
	}
	return leaf == "com.docker.backend"
}

// Get returns the current environment type.
func Get() Environment {
	environmentOnce.Do(func() {
		// Check if we're running in a Docker Desktop backend process.
		if executable, err := os.Executable(); err == nil && isDockerBackend(executable) {
			environment = EnvironmentDesktop
			return
		}

		// Look for a MODEL_RUNNER_ENVIRONMENT variable. If none is set or it's
		// invalid, then leave the environment unknown.
		switch os.Getenv("MODEL_RUNNER_ENVIRONMENT") {
		case "moby":
			environment = EnvironmentMoby
		case "cloud":
			environment = EnvironmentCloud
		}
	})
	return environment
}
