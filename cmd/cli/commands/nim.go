package commands

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	gpupkg "github.com/docker/model-runner/cmd/cli/pkg/gpu"
	"github.com/spf13/cobra"
)

const (
	// nimPrefix is the registry prefix for NVIDIA NIM images
	nimPrefix = "nvcr.io/nim/"
	// nimContainerPrefix is the prefix for NIM container names
	nimContainerPrefix = "docker-model-nim-"
	// nimDefaultPort is the default port for NIM containers
	nimDefaultPort = 8000
	// nimDefaultShmSize is the default shared memory size for NIM containers (16GB)
	nimDefaultShmSize = 17179869184
)

// isNIMImage checks if the given model reference is an NVIDIA NIM image
func isNIMImage(model string) bool {
	return strings.HasPrefix(model, nimPrefix)
}

// getDockerAuth retrieves authentication for a registry from Docker config
func getDockerAuth(registry string) (string, error) {
	// Get Docker config directory
	configDir := os.Getenv("DOCKER_CONFIG")
	if configDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		configDir = filepath.Join(homeDir, ".docker")
	}

	configFile := filepath.Join(configDir, "config.json")
	
	// Read Docker config file
	data, err := os.ReadFile(configFile)
	if err != nil {
		// If config file doesn't exist, return empty auth
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read Docker config: %w", err)
	}

	// Parse config JSON
	var config struct {
		Auths map[string]struct {
			Auth string `json:"auth"`
		} `json:"auths"`
		CredHelpers map[string]string `json:"credHelpers"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return "", fmt.Errorf("failed to parse Docker config: %w", err)
	}

	// Look for auth for the specific registry
	if auth, exists := config.Auths[registry]; exists && auth.Auth != "" {
		return auth.Auth, nil
	}

	// Also check for credential helpers (though this is more complex to implement)
	// For now, we'll just return empty if no direct auth is found
	return "", nil
}

// nimContainerName generates a container name for a NIM image
func nimContainerName(model string) string {
	// Extract the model name from the reference (e.g., nvcr.io/nim/google/gemma-3-1b-it:latest -> google-gemma-3-1b-it)
	parts := strings.Split(strings.TrimPrefix(model, nimPrefix), "/")
	name := strings.Join(parts, "-")
	// Remove tag if present
	if idx := strings.Index(name, ":"); idx != -1 {
		name = name[:idx]
	}
	// Replace any remaining special characters
	name = strings.ReplaceAll(name, ":", "-")
	name = strings.ReplaceAll(name, "/", "-")
	return nimContainerPrefix + name
}

// pullNIMImage pulls the NIM Docker image
func pullNIMImage(ctx context.Context, dockerClient *client.Client, model string, cmd *cobra.Command) error {
	cmd.Printf("Pulling NIM image %s...\n", model)

	// Debug: Show what we're looking for
	cmd.Printf("Looking for authentication for registry: nvcr.io\n")

	// Get authentication for nvcr.io
	authStr, err := getDockerAuth("nvcr.io")
	if err != nil {
		cmd.Printf("Warning: Failed to get Docker authentication: %v\n", err)
	}

	// Debug: Check what's in the Docker config (without showing credentials)
	if authStr == "" {
		cmd.Printf("Debug: No stored credentials found for nvcr.io. Checking Docker config...\n")
		configDir := os.Getenv("DOCKER_CONFIG")
		if configDir == "" {
			homeDir, _ := os.UserHomeDir()
			configDir = filepath.Join(homeDir, ".docker")
		}
		configFile := filepath.Join(configDir, "config.json")
		
		if data, err := os.ReadFile(configFile); err == nil {
			var config struct {
				Auths map[string]interface{} `json:"auths"`
			}
			if json.Unmarshal(data, &config) == nil {
				cmd.Printf("Debug: Found auth entries for: ")
				for registry := range config.Auths {
					cmd.Printf("%s ", registry)
				}
				cmd.Printf("\n")
			}
		}
	}

	pullOptions := image.PullOptions{}
	
	// Set authentication if available
	if authStr != "" {
		// Decode the base64 auth string from Docker config
		decoded, err := base64.StdEncoding.DecodeString(authStr)
		if err != nil {
			cmd.Printf("Warning: Failed to decode Docker auth string: %v\n", err)
		} else {
			parts := strings.SplitN(string(decoded), ":", 2)
			username, password := "", ""
			if len(parts) == 2 {
				username = parts[0]
				password = parts[1]
			}
			authJSON, _ := json.Marshal(map[string]string{
				"username": username,
				"password": password,
			})
			pullOptions.RegistryAuth = base64.StdEncoding.EncodeToString(authJSON)
			cmd.Println("Using stored Docker credentials for nvcr.io")
		}
	} else {
		// Try to use NGC_API_KEY as fallback authentication
		ngcAPIKey := os.Getenv("NGC_API_KEY")
		if ngcAPIKey != "" {
			// Create basic auth with NGC API key
			// For nvcr.io, username is "$oauthtoken" and password is the NGC API key
			auth := base64.StdEncoding.EncodeToString([]byte("$oauthtoken:" + ngcAPIKey))
			pullOptions.RegistryAuth = auth
			cmd.Println("Using NGC_API_KEY for authentication")
		} else {
			cmd.Println("Warning: No authentication found. You may need to run 'docker login nvcr.io' or set NGC_API_KEY environment variable")
		}
	}

	reader, err := dockerClient.ImagePull(ctx, model, pullOptions)
	if err != nil {
		if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "unauthorized") {
			return fmt.Errorf("authentication failed when pulling NIM image. Please ensure you have logged in with 'docker login nvcr.io' or set NGC_API_KEY environment variable: %w", err)
		}
		return fmt.Errorf("failed to pull NIM image: %w", err)
	}
	defer reader.Close()

	// Stream pull progress
	io.Copy(cmd.OutOrStdout(), reader)

	return nil
}

// findNIMContainer finds an existing NIM container for the given model
func findNIMContainer(ctx context.Context, dockerClient *client.Client, model string) (string, error) {
	containerName := nimContainerName(model)

	containers, err := dockerClient.ContainerList(ctx, container.ListOptions{
		All: true,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list containers: %w", err)
	}

	for _, c := range containers {
		for _, name := range c.Names {
			if strings.TrimPrefix(name, "/") == containerName {
				return c.ID, nil
			}
		}
	}

	return "", nil
}

// createNIMContainer creates and starts a NIM container
func createNIMContainer(ctx context.Context, dockerClient *client.Client, model string, cmd *cobra.Command) (string, error) {
	containerName := nimContainerName(model)

	// Get NGC API key from environment
	ngcAPIKey := os.Getenv("NGC_API_KEY")
	if ngcAPIKey == "" {
		cmd.Println("Warning: NGC_API_KEY environment variable is not set. NIM may require authentication.")
	}

	// Check for GPU support
	gpu, err := gpupkg.ProbeGPUSupport(ctx, dockerClient)
	if err != nil {
		cmd.Printf("Warning: Failed to probe GPU support: %v\n", err)
		gpu = gpupkg.GPUSupportNone
	}

	// Create cache directory
	cacheDir := os.Getenv("LOCAL_NIM_CACHE")
	if cacheDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		cacheDir = homeDir + "/.cache/nim"
	}

	// Create the cache directory if it doesn't exist
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create NIM cache directory: %w", err)
	}

	// Container configuration
	env := []string{}
	if ngcAPIKey != "" {
		env = append(env, "NGC_API_KEY="+ngcAPIKey)
	}

	portStr := strconv.Itoa(nimDefaultPort)
	config := &container.Config{
		Image: model,
		Env:   env,
		ExposedPorts: nat.PortSet{
			nat.Port(portStr + "/tcp"): struct{}{},
		},
	}

	hostConfig := &container.HostConfig{
		ShmSize: nimDefaultShmSize,
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: cacheDir,
				Target: "/opt/nim/.cache",
			},
		},
		PortBindings: nat.PortMap{
			nat.Port(portStr + "/tcp"): []nat.PortBinding{
				{
					HostIP:   "127.0.0.1",
					HostPort: portStr,
				},
			},
		},
	}

	// Add GPU support if available
	if gpu == gpupkg.GPUSupportCUDA {
		if ok, err := gpupkg.HasNVIDIARuntime(ctx, dockerClient); err == nil && ok {
			hostConfig.Runtime = "nvidia"
		}
		hostConfig.DeviceRequests = []container.DeviceRequest{{
			Count:        -1,
			Capabilities: [][]string{{"gpu"}},
		}}
	}

	// Create the container
	resp, err := dockerClient.ContainerCreate(ctx, config, hostConfig, nil, nil, containerName)
	if err != nil {
		return "", fmt.Errorf("failed to create NIM container: %w", err)
	}

	// Start the container
	if err := dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("failed to start NIM container: %w", err)
	}

	cmd.Printf("Started NIM container %s\n", containerName)
	if gpu == gpupkg.GPUSupportCUDA {
		cmd.Println("GPU support enabled")
	} else {
		cmd.Println("Warning: No GPU detected. NIM performance may be limited.")
	}

	return resp.ID, nil
}

// waitForNIMReady waits for the NIM container to be ready
func waitForNIMReady(ctx context.Context, cmd *cobra.Command) error {
	cmd.Println("Waiting for NIM to be ready (this may take several minutes)...")

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	maxRetries := 120 // 10 minutes with 5 second intervals
	for i := 0; i < maxRetries; i++ {
		resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/v1/models", nimDefaultPort))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				cmd.Println("NIM is ready!")
				return nil
			}
		}

		if i%12 == 0 { // Print status every minute
			elapsed := i * 5
			cmd.Printf("Still waiting for NIM to initialize... (%d seconds elapsed)\n", elapsed)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			// Continue waiting
		}
	}

	return fmt.Errorf("NIM failed to become ready within timeout. Check container logs with: docker logs $(docker ps -q --filter name=docker-model-nim-)")
}

// runNIMModel handles running an NVIDIA NIM image
func runNIMModel(ctx context.Context, dockerClient *client.Client, model string, cmd *cobra.Command) error {
	// Check if container already exists
	containerID, err := findNIMContainer(ctx, dockerClient, model)
	if err != nil {
		return err
	}

	if containerID != "" {
		// Container exists, check if it's running
		inspect, err := dockerClient.ContainerInspect(ctx, containerID)
		if err != nil {
			return fmt.Errorf("failed to inspect NIM container: %w", err)
		}

		if !inspect.State.Running {
			// Container exists but is not running, start it
			if err := dockerClient.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
				return fmt.Errorf("failed to start existing NIM container: %w", err)
			}
			cmd.Printf("Started existing NIM container %s\n", nimContainerName(model))
		} else {
			cmd.Printf("Using existing NIM container %s\n", nimContainerName(model))
		}
	} else {
		// Pull the image
		if err := pullNIMImage(ctx, dockerClient, model, cmd); err != nil {
			return err
		}

		// Create and start container
		containerID, err = createNIMContainer(ctx, dockerClient, model, cmd)
		if err != nil {
			return err
		}
	}

	// Wait for NIM to be ready
	if err := waitForNIMReady(ctx, cmd); err != nil {
		return err
	}

	return nil
}

// chatWithNIM sends chat requests to a NIM container
func chatWithNIM(cmd *cobra.Command, model, prompt string) error {
	// Use the desktop client to chat with the NIM through its OpenAI-compatible API
	// The NIM container runs on localhost:8000 and provides an OpenAI-compatible API

	// Create a simple HTTP client to talk to the NIM
	client := &http.Client{
		Timeout: 300 * time.Second,
	}

	// Build the request payload - use just the model base name without registry
	modelName := strings.TrimPrefix(model, nimPrefix)
	if idx := strings.LastIndex(modelName, ":"); idx != -1 {
		modelName = modelName[:idx]
	}

	reqBody := fmt.Sprintf(`{
		"model": "%s",
		"messages": [
			{"role": "user", "content": %q}
		],
		"stream": true
	}`, modelName, prompt)

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", nimDefaultPort), strings.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request to NIM: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("NIM returned error status %d: %s", resp.StatusCode, string(body))
	}

	// Stream the response - parse SSE events
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// SSE events start with "data: "
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")

			// Skip [DONE] message
			if data == "[DONE]" {
				continue
			}

			// Parse the JSON and extract the content
			// For simplicity, we'll use basic string parsing
			// In production, we'd use proper JSON parsing
			if strings.Contains(data, `"content"`) {
				// Extract content field - simple approach
				contentStart := strings.Index(data, `"content":"`)
				if contentStart != -1 {
					contentStart += len(`"content":"`)
					contentEnd := strings.Index(data[contentStart:], `"`)
					if contentEnd != -1 {
						content := data[contentStart : contentStart+contentEnd]
						// Unescape basic JSON escapes
						content = strings.ReplaceAll(content, `\n`, "\n")
						content = strings.ReplaceAll(content, `\t`, "\t")
						content = strings.ReplaceAll(content, `\"`, `"`)
						cmd.Print(content)
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading response: %w", err)
	}

	return nil
}
