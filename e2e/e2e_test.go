//go:build e2e

// Package e2e contains end-to-end tests that build and run the full
// model-runner stack (server + llama.cpp backend + CLI) from source.
//
// Run with:
//
//	go test -v -count=1 -tags=e2e -timeout=15m ./e2e/
package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/pkg/ollama"
)

const (
	// ggufModel is a small GGUF model for llama.cpp tests.
	ggufModel = "ai/smollm2:135M-Q4_0"
	// mlxModel is a small MLX-format model served by the vllm-metal backend.
	mlxModel = "huggingface.co/mlx-community/SmolLM2-135M-Instruct"

	serverStartTimeout = 60 * time.Second
)

var (
	serverURL string
	cliBin    string
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	root, err := filepath.Abs("..")
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: %v\n", err)
		return 1
	}
	cliBin = filepath.Join(root, "cmd", "cli", "model-cli")

	if runtime.GOOS == "darwin" {
		return runNative(m, root)
	}
	return runDocker(m, root)
}

// runNative builds the server from source and runs it as a local process.
func runNative(m *testing.M, root string) int {
	fmt.Fprintln(os.Stderr, "e2e: building llama.cpp, server, and CLI...")
	if err := makeTarget(root, "build-llamacpp"); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: make build-llamacpp failed: %v\n", err)
		return 1
	}
	if err := makeTarget(root, "build"); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: make build failed: %v\n", err)
		return 1
	}

	serverBin := filepath.Join(root, "model-runner")
	llamaBin := filepath.Join(root, "llamacpp", "install", "bin")

	for _, path := range []string{serverBin, cliBin, llamaBin} {
		if _, err := os.Stat(path); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: not found: %s\n", path)
			return 1
		}
	}

	port, err := freePort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: %v\n", err)
		return 1
	}
	serverURL = "http://localhost:" + strconv.Itoa(port)
	fmt.Fprintf(os.Stderr, "e2e: starting model-runner on port %d\n", port)

	ctx, cancel := context.WithCancel(context.Background())

	server := exec.CommandContext(ctx, serverBin)
	// TODO: os.Interrupt is not supported on Windows. When Windows e2e
	// tests are added, use a platform-specific shutdown mechanism.
	server.Cancel = func() error {
		return server.Process.Signal(os.Interrupt)
	}
	server.Dir = root
	server.Env = append(os.Environ(),
		"MODEL_RUNNER_PORT="+strconv.Itoa(port),
		"LLAMA_SERVER_PATH="+llamaBin,
	)
	server.Stdout = os.Stderr
	server.Stderr = os.Stderr

	if err := server.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: failed to start server: %v\n", err)
		cancel()
		return 1
	}

	code := waitAndRunTests(m)

	// Shut down the server and drain its goroutines before the leak check.
	cancel()
	_ = server.Wait()

	// Close idle keep-alive connections so the default HTTP transport does
	// not leave goroutines that would be false-positive leaks.
	http.DefaultClient.CloseIdleConnections()

	// Only check for leaks on a successful run; a failing run already
	// reports errors and extra leak noise adds no value.
	if code == 0 {
		if err := goleak.Find(); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: goroutine leak detected: %v\n", err)
			code = 1
		}
	}
	return code
}

// runDocker builds the Docker image and CLI from source, then lets the CLI
// auto-start the model-runner container on the default Moby port (12434).
func runDocker(m *testing.M, root string) int {
	fmt.Fprintln(os.Stderr, "e2e: building Docker image and CLI...")
	if err := makeTarget(root, "docker-build"); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: make docker-build failed: %v\n", err)
		return 1
	}
	if err := makeTarget(root, "build-cli"); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: make build-cli failed: %v\n", err)
		return 1
	}

	// Tag the locally built image so install-runner uses it
	// instead of pulling from Docker Hub.
	tag := exec.Command("docker", "tag", "docker/model-runner:latest", "docker/model-runner:e2e-local")
	if err := tag.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: docker tag failed: %v\n", err)
		return 1
	}

	fmt.Fprintln(os.Stderr, "e2e: installing runner...")
	cmd := exec.Command(cliBin, "install-runner")
	cmd.Env = append(os.Environ(), "MODEL_RUNNER_CONTROLLER_VERSION=e2e-local")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: install-runner failed: %v\n", err)
		return 1
	}

	serverURL = "http://localhost:12434"
	code := waitAndRunTests(m)

	// Close idle keep-alive connections so the default HTTP transport does
	// not leave goroutines that would be false-positive leaks.
	http.DefaultClient.CloseIdleConnections()

	// Only check for leaks on a successful run; a failing run already
	// reports errors and extra leak noise adds no value.
	if code == 0 {
		if err := goleak.Find(); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: goroutine leak detected: %v\n", err)
			code = 1
		}
	}
	return code
}

func waitAndRunTests(m *testing.M) int {
	if err := waitForServer(serverURL+"/models", serverStartTimeout); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "e2e: server ready at %s\n", serverURL)
	return m.Run()
}

func makeTarget(dir, target string) error {
	cmd := exec.Command("make", target)
	cmd.Dir = dir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("finding free port: %w", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func waitForServer(url string, timeout time.Duration) error {
	client := &http.Client{Timeout: 2 * time.Second}
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

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), cliBin, args...)
	cmd.Env = append(os.Environ(), "MODEL_RUNNER_HOST="+serverURL)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func pullModel(t *testing.T, model string) string {
	t.Helper()
	status, body := doJSON(t, http.MethodPost, serverURL+"/models/create",
		map[string]string{"from": model})
	if status != http.StatusOK && status != http.StatusCreated {
		t.Fatalf("pull %s failed: status=%d body=%s", model, status, body)
	}
	return string(body)
}

// removeModel removes a model via API. Does not fail if the model doesn't exist.
func removeModel(t *testing.T, model string) {
	t.Helper()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodDelete,
		fmt.Sprintf("%s/models/%s", serverURL, model), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

// doJSON sends a JSON request and returns the response body.
func doJSON(t *testing.T, method, url string, body any) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, url, reader)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request %s %s failed: %v", method, url, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body for %s %s: %v", method, url, err)
	}
	return resp.StatusCode, respBody
}

// readSSE reads an SSE stream and returns accumulated content and chunk count.
func readSSE(t *testing.T, resp *http.Response) (content string, chunks int, gotDone bool) {
	t.Helper()
	var accumulated strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			t.Fatalf("unexpected SSE line: %q", line)
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			gotDone = true
			break
		}
		var chunk desktop.OpenAIChatResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			t.Fatalf("parsing SSE chunk: %v (data=%q)", err, data)
		}
		chunks++
		if len(chunk.Choices) > 0 {
			accumulated.WriteString(chunk.Choices[0].Delta.Content)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("reading stream: %v", err)
	}
	return accumulated.String(), chunks, gotDone
}

// chatCompletion sends a non-streaming chat request and returns the response.
func chatCompletion(t *testing.T, model, prompt string) desktop.OpenAIChatResponse {
	t.Helper()
	status, body := doJSON(t, http.MethodPost, serverURL+"/engines/v1/chat/completions",
		desktop.OpenAIChatRequest{
			Model:    model,
			Messages: []desktop.OpenAIChatMessage{{Role: "user", Content: prompt}},
			Stream:   false,
		})
	if status != http.StatusOK {
		t.Fatalf("chat completion failed: status=%d body=%s", status, body)
	}
	var resp desktop.OpenAIChatResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decoding response: %v (body=%s)", err, body)
	}
	return resp
}

// streamingChatCompletion sends a streaming chat request and validates the SSE response.
func streamingChatCompletion(t *testing.T, model, prompt string) string {
	t.Helper()
	data, err := json.Marshal(desktop.OpenAIChatRequest{
		Model:    model,
		Messages: []desktop.OpenAIChatMessage{{Role: "user", Content: prompt}},
		Stream:   true,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		serverURL+"/engines/v1/chat/completions", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			t.Fatalf("streaming failed: status=%d (could not read body: %v)", resp.StatusCode, readErr)
		}
		t.Fatalf("streaming failed: status=%d body=%s", resp.StatusCode, body)
	}

	content, chunks, gotDone := readSSE(t, resp)
	if !gotDone {
		t.Error("stream did not end with [DONE]")
	}
	if chunks == 0 {
		t.Error("received no SSE chunks")
	}
	if content == "" {
		t.Error("accumulated content is empty")
	}
	return content
}

// ollamaChat sends an Ollama-compatible /api/chat request (non-streaming).
func ollamaChat(t *testing.T, model, prompt string) ollama.ChatResponse {
	t.Helper()
	stream := false
	status, body := doJSON(t, http.MethodPost, serverURL+"/api/chat",
		ollama.ChatRequest{
			Model:    model,
			Messages: []ollama.Message{{Role: "user", Content: prompt}},
			Stream:   &stream,
		})
	if status != http.StatusOK {
		t.Fatalf("ollama chat failed: status=%d body=%s", status, body)
	}
	var resp ollama.ChatResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decoding ollama response: %v (body=%s)", err, body)
	}
	return resp
}

// ollamaGenerate sends an Ollama-compatible /api/generate request (non-streaming).
func ollamaGenerate(t *testing.T, model, prompt string) ollama.GenerateResponse {
	t.Helper()
	stream := false
	status, body := doJSON(t, http.MethodPost, serverURL+"/api/generate",
		ollama.GenerateRequest{
			Model:  model,
			Prompt: prompt,
			Stream: &stream,
		})
	if status != http.StatusOK {
		t.Fatalf("ollama generate failed: status=%d body=%s", status, body)
	}
	var resp ollama.GenerateResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decoding ollama response: %v (body=%s)", err, body)
	}
	return resp
}
