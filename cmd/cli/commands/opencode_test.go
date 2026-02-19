package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/cmd/cli/pkg/standalone"
	"github.com/docker/model-runner/cmd/cli/pkg/types"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/stretchr/testify/require"
)

func TestDetermineHostPortDesktop(t *testing.T) {
	ctx, err := desktop.NewContextForTest(
		"http://localhost"+inference.ExperimentalEndpointsPrefix,
		nil,
		types.ModelRunnerEngineKindDesktop,
	)
	require.NoError(t, err)
	modelRunner = ctx

	// For Desktop, always returns default port regardless of runner
	port := determineHostPort(nil)
	require.Equal(t, uint16(standalone.DefaultControllerPortMoby), port)

	// Even with a runner, Desktop uses default port
	runner := &standaloneRunner{
		hostPort: 9999,
	}
	port = determineHostPort(runner)
	require.Equal(t, uint16(standalone.DefaultControllerPortMoby), port)
}

func TestDetermineHostPortCloudWithGateway(t *testing.T) {
	ctx, err := desktop.NewContextForTest(
		"http://localhost:12435",
		nil,
		types.ModelRunnerEngineKindCloud,
	)
	require.NoError(t, err)
	modelRunner = ctx

	runner := &standaloneRunner{
		gatewayIP:   "172.17.0.1",
		gatewayPort: 12435,
	}
	port := determineHostPort(runner)
	require.Equal(t, uint16(12435), port)
}

func TestDetermineHostPortMobyWithHostPort(t *testing.T) {
	ctx, err := desktop.NewContextForTest(
		"http://localhost:12434",
		nil,
		types.ModelRunnerEngineKindMoby,
	)
	require.NoError(t, err)
	modelRunner = ctx

	runner := &standaloneRunner{
		hostPort: 12434,
	}
	port := determineHostPort(runner)
	require.Equal(t, uint16(12434), port)
}

func TestDetermineHostPortFallback(t *testing.T) {
	ctx, err := desktop.NewContextForTest(
		"http://localhost:12434",
		nil,
		types.ModelRunnerEngineKindMoby,
	)
	require.NoError(t, err)
	modelRunner = ctx

	// Nil runner should fallback to default
	port := determineHostPort(nil)
	require.Equal(t, uint16(standalone.DefaultControllerPortMoby), port)

	// Runner with no ports should fallback to default
	runner := &standaloneRunner{
		gatewayIP:   "172.17.0.1",
		gatewayPort: 0,
		hostPort:    0,
	}
	port = determineHostPort(runner)
	require.Equal(t, uint16(standalone.DefaultControllerPortMoby), port)
}

func TestWriteOpenCodeConfig(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Mock home directory
	t.Setenv("HOME", tmpDir)

	ctx, err := desktop.NewContextForTest(
		"http://localhost"+inference.ExperimentalEndpointsPrefix,
		nil,
		types.ModelRunnerEngineKindDesktop,
	)
	require.NoError(t, err)
	modelRunner = ctx

	runner := &standaloneRunner{
		hostPort: 12434,
	}

	model := "ai/test-model"
	err = writeOpenCodeConfig(runner, model)
	require.NoError(t, err)

	// Verify config file was created
	configPath := filepath.Join(tmpDir, ".config", "opencode", "opencode.json")
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var config map[string]any
	err = json.Unmarshal(data, &config)
	require.NoError(t, err)

	// Verify schema
	require.Equal(t, "https://opencode.ai/config.json", config["$schema"])

	// Verify provider configuration
	provider, ok := config["provider"].(map[string]any)
	require.True(t, ok)

	dmr, ok := provider["dmr"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "@ai-sdk/openai-compatible", dmr["npm"])
	require.Equal(t, "Docker Model Runner (local)", dmr["name"])

	options, ok := dmr["options"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "http://127.0.0.1:12434/v1", options["baseURL"])

	// Verify model configuration
	models, ok := dmr["models"].(map[string]any)
	require.True(t, ok)

	modelEntry, ok := models[model].(map[string]any)
	require.True(t, ok)
	require.Equal(t, model, modelEntry["name"])
	require.Equal(t, true, modelEntry["_launch"])
}

func TestWriteOpenCodeConfigUpdatesExisting(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Mock home directory
	t.Setenv("HOME", tmpDir)

	ctx, err := desktop.NewContextForTest(
		"http://localhost"+inference.ExperimentalEndpointsPrefix,
		nil,
		types.ModelRunnerEngineKindDesktop,
	)
	require.NoError(t, err)
	modelRunner = ctx

	// Create existing config
	configDir := filepath.Join(tmpDir, ".config", "opencode")
	err = os.MkdirAll(configDir, 0o755)
	require.NoError(t, err)

	configPath := filepath.Join(configDir, "opencode.json")
	existingConfig := map[string]any{
		"existing": "value",
		"provider": map[string]any{
			"other": map[string]any{
				"key": "value",
			},
		},
	}
	data, err := json.MarshalIndent(existingConfig, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(configPath, data, 0o644)
	require.NoError(t, err)

	// For Desktop engine kind, the port is always the default
	runner := &standaloneRunner{
		hostPort: 9999,
	}

	model := "ai/new-model"
	err = writeOpenCodeConfig(runner, model)
	require.NoError(t, err)

	// Verify config was updated
	data, err = os.ReadFile(configPath)
	require.NoError(t, err)

	var config map[string]any
	err = json.Unmarshal(data, &config)
	require.NoError(t, err)

	// Verify existing values are preserved
	require.Equal(t, "value", config["existing"])

	provider, ok := config["provider"].(map[string]any)
	require.True(t, ok)

	// Verify other provider is preserved
	other, ok := provider["other"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "value", other["key"])

	// Verify dmr was added/updated
	dmr, ok := provider["dmr"].(map[string]any)
	require.True(t, ok)
	options, ok := dmr["options"].(map[string]any)
	require.True(t, ok)
	// For Desktop, uses default port regardless of runner port
	require.Equal(t, "http://127.0.0.1:12434/v1", options["baseURL"])

	// Verify new model was added
	models, ok := dmr["models"].(map[string]any)
	require.True(t, ok)
	_, ok = models[model].(map[string]any)
	require.True(t, ok)
}

func TestWriteOpenCodeConfigCreatesDirectory(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Mock home directory
	t.Setenv("HOME", tmpDir)

	ctx, err := desktop.NewContextForTest(
		"http://localhost"+inference.ExperimentalEndpointsPrefix,
		nil,
		types.ModelRunnerEngineKindDesktop,
	)
	require.NoError(t, err)
	modelRunner = ctx

	runner := &standaloneRunner{
		hostPort: 12434,
	}

	model := "ai/test-model"
	err = writeOpenCodeConfig(runner, model)
	require.NoError(t, err)

	// Verify directory was created
	configDir := filepath.Join(tmpDir, ".config", "opencode")
	_, err = os.Stat(configDir)
	require.NoError(t, err)
}

func TestWriteOpenCodeState(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Mock home directory
	t.Setenv("HOME", tmpDir)

	model := "ai/test-model"
	err := writeOpenCodeState(model)
	require.NoError(t, err)

	// Verify state file was created
	statePath := filepath.Join(tmpDir, ".local", "state", "opencode", "model.json")
	data, err := os.ReadFile(statePath)
	require.NoError(t, err)

	var state map[string]any
	err = json.Unmarshal(data, &state)
	require.NoError(t, err)

	// Verify recent models
	recent, ok := state["recent"].([]any)
	require.True(t, ok)
	require.Len(t, recent, 1)

	entry, ok := recent[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "dmr", entry["providerID"])
	require.Equal(t, model, entry["modelID"])

	// Verify favorite and variant are empty/default
	favorite, ok := state["favorite"].([]any)
	require.True(t, ok)
	require.Empty(t, favorite)

	variant, ok := state["variant"].(map[string]any)
	require.True(t, ok)
	require.Empty(t, variant)
}

func TestWriteOpenCodeStatePrependsNewModel(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Mock home directory
	t.Setenv("HOME", tmpDir)

	// Create existing state
	stateDir := filepath.Join(tmpDir, ".local", "state", "opencode")
	err := os.MkdirAll(stateDir, 0o755)
	require.NoError(t, err)

	statePath := filepath.Join(stateDir, "model.json")
	existingState := map[string]any{
		"recent": []any{
			map[string]any{
				"providerID": "dmr",
				"modelID":    "ai/old-model",
			},
			map[string]any{
				"providerID": "other",
				"modelID":    "other-model",
			},
		},
	}
	data, err := json.MarshalIndent(existingState, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(statePath, data, 0o644)
	require.NoError(t, err)

	newModel := "ai/new-model"
	err = writeOpenCodeState(newModel)
	require.NoError(t, err)

	// Verify state was updated
	data, err = os.ReadFile(statePath)
	require.NoError(t, err)

	var state map[string]any
	err = json.Unmarshal(data, &state)
	require.NoError(t, err)

	recent, ok := state["recent"].([]any)
	require.True(t, ok)
	require.Len(t, recent, 3)

	// Verify new model is first
	firstEntry, ok := recent[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "dmr", firstEntry["providerID"])
	require.Equal(t, newModel, firstEntry["modelID"])

	// Verify old dmr model is still there but after the new one
	secondEntry, ok := recent[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "dmr", secondEntry["providerID"])
	require.Equal(t, "ai/old-model", secondEntry["modelID"])
}

func TestWriteOpenCodeStateRemovesDuplicate(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Mock home directory
	t.Setenv("HOME", tmpDir)

	// Create existing state with duplicate model
	stateDir := filepath.Join(tmpDir, ".local", "state", "opencode")
	err := os.MkdirAll(stateDir, 0o755)
	require.NoError(t, err)

	statePath := filepath.Join(stateDir, "model.json")
	existingState := map[string]any{
		"recent": []any{
			map[string]any{
				"providerID": "dmr",
				"modelID":    "ai/existing-model",
			},
			map[string]any{
				"providerID": "other",
				"modelID":    "other-model",
			},
		},
	}
	data, err := json.MarshalIndent(existingState, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(statePath, data, 0o644)
	require.NoError(t, err)

	existingModel := "ai/existing-model"
	err = writeOpenCodeState(existingModel)
	require.NoError(t, err)

	// Verify state was updated (no duplicates)
	data, err = os.ReadFile(statePath)
	require.NoError(t, err)

	var state map[string]any
	err = json.Unmarshal(data, &state)
	require.NoError(t, err)

	recent, ok := state["recent"].([]any)
	require.True(t, ok)
	require.Len(t, recent, 2)

	// Verify existing model is first (moved to top)
	firstEntry, ok := recent[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "dmr", firstEntry["providerID"])
	require.Equal(t, existingModel, firstEntry["modelID"])

	// Verify other model is still there
	secondEntry, ok := recent[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "other", secondEntry["providerID"])
}

func TestWriteOpenCodeStateLimitsToMaxRecent(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Mock home directory
	t.Setenv("HOME", tmpDir)

	// Create existing state with many models
	stateDir := filepath.Join(tmpDir, ".local", "state", "opencode")
	err := os.MkdirAll(stateDir, 0o755)
	require.NoError(t, err)

	statePath := filepath.Join(stateDir, "model.json")

	// Create 15 existing models
	existingRecent := make([]any, 15)
	for i := 0; i < 15; i++ {
		existingRecent[i] = map[string]any{
			"providerID": "dmr",
			"modelID":    "ai/model-" + string(rune('a'+i)),
		}
	}

	existingState := map[string]any{
		"recent": existingRecent,
	}
	data, err := json.MarshalIndent(existingState, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(statePath, data, 0o644)
	require.NoError(t, err)

	newModel := "ai/new-model"
	err = writeOpenCodeState(newModel)
	require.NoError(t, err)

	// Verify state was limited to max
	data, err = os.ReadFile(statePath)
	require.NoError(t, err)

	var state map[string]any
	err = json.Unmarshal(data, &state)
	require.NoError(t, err)

	recent, ok := state["recent"].([]any)
	require.True(t, ok)
	require.Len(t, recent, 10)

	// Verify new model is first
	firstEntry, ok := recent[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "dmr", firstEntry["providerID"])
	require.Equal(t, newModel, firstEntry["modelID"])
}

func TestWriteOpenCodeStateHandlesCorruptFile(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Mock home directory
	t.Setenv("HOME", tmpDir)

	// Create corrupt state file
	stateDir := filepath.Join(tmpDir, ".local", "state", "opencode")
	err := os.MkdirAll(stateDir, 0o755)
	require.NoError(t, err)

	statePath := filepath.Join(stateDir, "model.json")
	err = os.WriteFile(statePath, []byte("not valid json"), 0o644)
	require.NoError(t, err)

	model := "ai/test-model"
	err = writeOpenCodeState(model)
	require.NoError(t, err)

	// Verify state was written with defaults
	data, err := os.ReadFile(statePath)
	require.NoError(t, err)

	var state map[string]any
	err = json.Unmarshal(data, &state)
	require.NoError(t, err)

	recent, ok := state["recent"].([]any)
	require.True(t, ok)
	require.Len(t, recent, 1)
}

func TestWriteOpenCodeStateHandlesMissingFile(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Mock home directory
	t.Setenv("HOME", tmpDir)

	model := "ai/test-model"
	err := writeOpenCodeState(model)
	require.NoError(t, err)

	// Verify state was written
	statePath := filepath.Join(tmpDir, ".local", "state", "opencode", "model.json")
	_, err = os.Stat(statePath)
	require.NoError(t, err)
}

func TestWriteOpenCodeStatePreservesNonDmrModels(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Mock home directory
	t.Setenv("HOME", tmpDir)

	// Create existing state with non-dmr models
	stateDir := filepath.Join(tmpDir, ".local", "state", "opencode")
	err := os.MkdirAll(stateDir, 0o755)
	require.NoError(t, err)

	statePath := filepath.Join(stateDir, "model.json")
	existingState := map[string]any{
		"recent": []any{
			map[string]any{
				"providerID": "other-provider",
				"modelID":    "other-model",
			},
		},
	}
	data, err := json.MarshalIndent(existingState, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(statePath, data, 0o644)
	require.NoError(t, err)

	model := "ai/test-model"
	err = writeOpenCodeState(model)
	require.NoError(t, err)

	// Verify state was updated
	data, err = os.ReadFile(statePath)
	require.NoError(t, err)

	var state map[string]any
	err = json.Unmarshal(data, &state)
	require.NoError(t, err)

	recent, ok := state["recent"].([]any)
	require.True(t, ok)
	require.Len(t, recent, 2)

	// Verify non-dmr model is preserved
	foundNonDmr := false
	for _, entry := range recent {
		e, ok := entry.(map[string]any)
		require.True(t, ok)
		if e["providerID"] == "other-provider" {
			foundNonDmr = true
			require.Equal(t, "other-model", e["modelID"])
		}
	}
	require.True(t, foundNonDmr)
}

func TestLaunchOpenCodeDryRun(t *testing.T) {
	ctx, err := desktop.NewContextForTest(
		"http://localhost"+inference.ExperimentalEndpointsPrefix,
		nil,
		types.ModelRunnerEngineKindDesktop,
	)
	require.NoError(t, err)
	modelRunner = ctx

	buf := new(bytes.Buffer)
	cmd := newTestCmd(buf)

	runner := &standaloneRunner{
		hostPort: 12434,
	}

	err = launchOpenCode(cmd, testBaseURL, "ai/test-model", runner, []string{"--help"}, true)
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "Would run: opencode --help")
	require.Contains(t, output, "OPENAI_API_BASE="+testBaseURL+"/engines/v1")
	require.Contains(t, output, "Would configure opencode with model: ai/test-model")
}

func TestLaunchOpenCodeDryRunNoModel(t *testing.T) {
	ctx, err := desktop.NewContextForTest(
		"http://localhost"+inference.ExperimentalEndpointsPrefix,
		nil,
		types.ModelRunnerEngineKindDesktop,
	)
	require.NoError(t, err)
	modelRunner = ctx

	buf := new(bytes.Buffer)
	cmd := newTestCmd(buf)

	runner := &standaloneRunner{
		hostPort: 12434,
	}

	err = launchOpenCode(cmd, testBaseURL, "", runner, nil, true)
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "Would run: opencode")
	require.NotContains(t, output, "Would configure opencode with model")
}
