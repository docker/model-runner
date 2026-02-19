package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/docker/model-runner/cmd/cli/pkg/standalone"
	"github.com/docker/model-runner/cmd/cli/pkg/types"
	"github.com/spf13/cobra"
)

// determineHostPort determines the port to use for host access to model-runner.
//
// The function uses the following logic:
//   - For Desktop engine kind: returns the default port (modelRunner global is used)
//   - For Cloud/Moby engine kinds: uses the port from the runner parameter if available
//   - Falls back to default port if no port is found
//
// Note: The runner parameter is only used for Cloud/Moby engine kinds.
// For Desktop engine kinds, the global modelRunner context is used instead.
func determineHostPort(runner *standaloneRunner) uint16 {
	kind := modelRunner.EngineKind()

	// For Desktop, use the default port
	if kind == types.ModelRunnerEngineKindDesktop {
		return standalone.DefaultControllerPortMoby
	}

	// For Cloud/Moby, use the actual port from the runner
	if runner != nil && runner.hostPort != 0 {
		return runner.hostPort
	}

	if runner != nil && runner.gatewayPort != 0 {
		return runner.gatewayPort
	}

	// Fallback to default
	return standalone.DefaultControllerPortMoby
}

// setupOpenCode writes the opencode.json and model.json configuration files.
func setupOpenCode(cmd *cobra.Command, runner *standaloneRunner, model string) error {
	// Check if model exists, pull if not
	if err := ensureModelExists(cmd, model); err != nil {
		return err
	}

	// Write opencode.json config
	if err := writeOpenCodeConfig(runner, model); err != nil {
		return fmt.Errorf("failed to write opencode config: %w", err)
	}

	// Write model.json state
	if err := writeOpenCodeState(model); err != nil {
		return fmt.Errorf("failed to write opencode state: %w", err)
	}

	return nil
}

// ensureModelExists checks if the model exists locally, and pulls it if not.
func ensureModelExists(cmd *cobra.Command, model string) error {
	models, err := desktopClient.List()
	if err != nil {
		return fmt.Errorf("failed to list models: %w", err)
	}

	// Check if model exists
	modelExists := false
	for _, m := range models {
		for _, tag := range m.Tags {
			if tag == model || strings.TrimPrefix(tag, "ai/") == model {
				modelExists = true
				break
			}
		}
		if modelExists {
			break
		}
	}

	if !modelExists {
		cmd.Printf("Model %s not found locally. Pulling...\n", model)
		if err := pullModel(cmd, desktopClient, model); err != nil {
			return fmt.Errorf("failed to pull model: %w", err)
		}
	}

	return nil
}

// writeOpenCodeConfig writes the ~/.config/opencode/opencode.json file.
func writeOpenCodeConfig(runner *standaloneRunner, model string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}

	config := make(map[string]any)
	if data, err := os.ReadFile(configPath); err == nil {
		_ = json.Unmarshal(data, &config) // Ignore parse errors; treat missing/corrupt files as empty
	}

	config["$schema"] = "https://opencode.ai/config.json"

	provider, ok := config["provider"].(map[string]any)
	if !ok {
		provider = make(map[string]any)
	}

	// Determine the correct port for host access
	port := determineHostPort(runner)
	opencodeBaseURL := fmt.Sprintf("http://127.0.0.1:%d/v1", port)

	dmr, ok := provider["dmr"].(map[string]any)
	if !ok {
		dmr = map[string]any{
			"npm":  "@ai-sdk/openai-compatible",
			"name": "Docker Model Runner (local)",
			"options": map[string]any{
				"baseURL": opencodeBaseURL,
			},
		}
	} else {
		// Update baseURL in existing config
		if options, ok := dmr["options"].(map[string]any); ok {
			options["baseURL"] = opencodeBaseURL
		} else {
			dmr["options"] = map[string]any{
				"baseURL": opencodeBaseURL,
			}
		}
	}

	models, ok := dmr["models"].(map[string]any)
	if !ok {
		models = make(map[string]any)
	}

	// Add model entry
	entry := map[string]any{
		"name":    model,
		"_launch": true,
	}
	models[model] = entry

	dmr["models"] = models
	provider["dmr"] = dmr
	config["provider"] = provider

	configData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, configData, 0o644)
}

// writeOpenCodeState writes the ~/.local/state/opencode/model.json file.
func writeOpenCodeState(model string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	statePath := filepath.Join(home, ".local", "state", "opencode", "model.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return err
	}

	state := map[string]any{
		"recent":   []any{},
		"favorite": []any{},
		"variant":  map[string]any{},
	}
	if data, err := os.ReadFile(statePath); err == nil {
		_ = json.Unmarshal(data, &state) // Ignore parse errors; use defaults
	}

	recent, _ := state["recent"].([]any)

	// Remove existing entry for this model if present
	newRecent := []any{}
	for _, entry := range recent {
		e, ok := entry.(map[string]any)
		if !ok || e["providerID"] != "dmr" {
			newRecent = append(newRecent, entry)
			continue
		}
		modelID, ok := e["modelID"].(string)
		if !ok || modelID != model {
			newRecent = append(newRecent, entry)
		}
	}

	// Prepend the new model
	newRecent = append([]any{
		map[string]any{
			"providerID": "dmr",
			"modelID":    model,
		},
	}, newRecent...)

	// Keep only the most recent 10 models
	const maxRecentModels = 10
	if len(newRecent) > maxRecentModels {
		newRecent = newRecent[:maxRecentModels]
	}

	state["recent"] = newRecent

	stateData, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(statePath, stateData, 0o644)
}

// launchOpenCode launches opencode with automatic configuration.
// It handles model verification, config file creation, and state management.
func launchOpenCode(cmd *cobra.Command, baseURL string, model string, runner *standaloneRunner, appArgs []string, dryRun bool) error {
	if !dryRun {
		if _, err := exec.LookPath("opencode"); err != nil {
			cmd.PrintErrf("%q executable not found in PATH.\n", "opencode")
			cmd.PrintErrf("Configure your app to use:\n")
			env := openaiEnv(openaiPathSuffix)(baseURL)
			for _, e := range env {
				cmd.PrintErrf("  %s\n", e)
			}
			return fmt.Errorf("opencode not found; please install it and re-run")
		}
	}

	// Setup opencode configuration (skip in dry-run mode)
	if model != "" && !dryRun {
		if err := setupOpenCode(cmd, runner, model); err != nil {
			return fmt.Errorf("failed to setup opencode: %w", err)
		}
	}

	env := openaiEnv(openaiPathSuffix)(baseURL)
	if dryRun {
		cmd.Printf("Would run: opencode %s\n", strings.Join(appArgs, " "))
		for _, e := range env {
			cmd.Printf("  %s\n", e)
		}
		if model != "" {
			cmd.Printf("Would configure opencode with model: %s\n", model)
		}
		return nil
	}
	return runExternal(cmd, withEnv(env...), "opencode", appArgs...)
}
