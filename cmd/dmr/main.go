// dmr is a developer convenience wrapper that starts the model-runner server on
// a free port and runs a model-cli command against it in one step.
//
// Usage: dmr <cli-args...>
//
// Example: dmr run qwen3:0.6B-Q4_0 tell me today's news
package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func waitForServer(url string, timeout time.Duration) error {
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("server not ready after %s", timeout)
}

func checkBinary(path, name, expectedLayout string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("missing %s binary at %s\n\nExpected directory layout:\n%s\n\nPlease run 'make build' to build all binaries", name, path, expectedLayout)
	}
	return nil
}

func main() {
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "dmr: %v\n", err)
		os.Exit(1)
	}
	dir := filepath.Dir(self)

	serverBin := filepath.Join(dir, "model-runner")
	cliBin := filepath.Join(dir, "cmd", "cli", "model-cli")

	expectedLayout := fmt.Sprintf(`%s/
├── model-runner          (server binary)
├── dmr                   (this wrapper)
└── cmd/
    └── cli/
        └── model-cli     (CLI binary)`, dir)

	if err := checkBinary(serverBin, "model-runner", expectedLayout); err != nil {
		fmt.Fprintf(os.Stderr, "dmr: %v\n", err)
		os.Exit(1)
	}
	if err := checkBinary(cliBin, "model-cli", expectedLayout); err != nil {
		fmt.Fprintf(os.Stderr, "dmr: %v\n", err)
		os.Exit(1)
	}

	port, err := freePort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "dmr: failed to find free port: %v\n", err)
		os.Exit(1)
	}
	portStr := strconv.Itoa(port)
	serverURL := "http://localhost:" + portStr

	fmt.Fprintf(os.Stderr, "dmr: starting model-runner on port %d\n", port)

	server := exec.Command(serverBin)
	server.Env = append(os.Environ(), "MODEL_RUNNER_PORT="+portStr)
	server.Stderr = os.Stderr
	server.Stdout = os.Stdout

	if err := server.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "dmr: failed to start model-runner: %v\n", err)
		os.Exit(1)
	}
	defer server.Process.Kill()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		server.Process.Kill()
	}()

	if err := waitForServer(serverURL+"/", 30*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "dmr: %v\n", err)
		os.Exit(1)
	}

	// #nosec G702 - Intentional: dmr is a CLI wrapper that forwards arguments to model-cli
	cli := exec.Command(cliBin, os.Args[1:]...)
	cli.Env = append(os.Environ(), "MODEL_RUNNER_HOST="+serverURL)
	cli.Stdin = os.Stdin
	cli.Stdout = os.Stdout
	cli.Stderr = os.Stderr

	if err := cli.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "dmr: %v\n", err)
		os.Exit(1)
	}
}
