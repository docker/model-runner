package commands

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/docker/model-runner/cmd/cli/pkg/modelctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupContextTest creates an isolated context store for a single test. It
// sets DOCKER_CONFIG to a temporary directory and clears dockerCLI so that
// dockerConfigDir() falls back to the env var rather than the real CLI
// config, keeping tests hermetic.
func setupContextTest(t *testing.T) *modelctx.Store {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dir)
	dockerCLI = nil // force dockerConfigDir() to use DOCKER_CONFIG

	store, err := modelctx.New(dir)
	require.NoError(t, err)
	return store
}

// TestContextCreate verifies that "context create" writes the context and
// prints a confirmation message.
func TestContextCreate(t *testing.T) {
	setupContextTest(t)

	cmd := newContextCreateCmd()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"myremote", "--host", "http://192.168.1.100:12434", "--description", "lab"})

	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), `"myremote"`)

	// Verify the context was actually stored.
	store, err := contextStore()
	require.NoError(t, err)
	cfg, err := store.Get("myremote")
	require.NoError(t, err)
	assert.Equal(t, "http://192.168.1.100:12434", cfg.Host)
	assert.Equal(t, "lab", cfg.Description)
}

// TestContextCreate_missingHost verifies that --host is required.
func TestContextCreate_missingHost(t *testing.T) {
	setupContextTest(t)

	cmd := newContextCreateCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"myremote"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--host")
}

// TestContextCreate_invalidName verifies that a name starting with a dash is
// rejected.
func TestContextCreate_invalidName(t *testing.T) {
	setupContextTest(t)

	cmd := newContextCreateCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"-badname", "--host", "http://localhost:12434"})

	// Cobra itself will reject args beginning with "-" as flag-like, so we
	// test with a name that passes Cobra but fails our validation.
	cmd2 := newContextCreateCmd()
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.SetErr(new(bytes.Buffer))
	cmd2.SetArgs([]string{"has space", "--host", "http://localhost:12434"})
	err := cmd2.Execute()
	require.Error(t, err)
}

// TestContextCreate_invalidHostURL verifies that hosts without a proper
// scheme or host component are rejected early.
func TestContextCreate_invalidHostURL(t *testing.T) {
	setupContextTest(t)

	tests := []struct {
		name string
		host string
		want string
	}{
		{"no scheme", "192.168.1.100:12434", "invalid --host URL"},
		{"bare word", "localhost", "invalid --host URL"},
		{"ftp scheme", "ftp://example.com:12434", "unsupported scheme"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newContextCreateCmd()
			cmd.SetOut(new(bytes.Buffer))
			cmd.SetErr(new(bytes.Buffer))
			cmd.SetArgs([]string{"test", "--host", tt.host})

			err := cmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// TestContextCreate_reservedName verifies that "default" is rejected.
func TestContextCreate_reservedName(t *testing.T) {
	setupContextTest(t)

	cmd := newContextCreateCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"default", "--host", "http://localhost:12434"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reserved")
}

// TestContextCreate_duplicate verifies that creating a context that already
// exists returns an error.
func TestContextCreate_duplicate(t *testing.T) {
	setupContextTest(t)

	for range 2 {
		cmd := newContextCreateCmd()
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetErr(new(bytes.Buffer))
		cmd.SetArgs([]string{"myremote", "--host", "http://localhost:12434"})
		_ = cmd.Execute()
	}

	cmd := newContextCreateCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"myremote", "--host", "http://localhost:12434"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

// TestContextLs_empty verifies that "context ls" always shows the "default"
// row even when no named contexts exist.
func TestContextLs_empty(t *testing.T) {
	setupContextTest(t)

	cmd := newContextLsCmd()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))

	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "default")
	assert.Contains(t, out.String(), "*") // default is active
}

// TestContextLs_withContexts verifies that named contexts appear in the list
// and that the active one is marked with "*".
func TestContextLs_withContexts(t *testing.T) {
	store := setupContextTest(t)
	require.NoError(t, store.Create("remote", modelctx.ContextConfig{
		Host:        "http://remote:12434",
		Description: "remote box",
	}))
	require.NoError(t, store.SetActive("remote"))

	cmd := newContextLsCmd()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))

	require.NoError(t, cmd.Execute())
	output := out.String()
	assert.Contains(t, output, "remote")
	assert.Contains(t, output, "http://remote:12434")

	// "remote" should be marked active; "default" should not have "*".
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "remote") && !strings.Contains(line, "default") {
			assert.Contains(t, line, "*", "active context should show *")
		}
		if strings.Contains(line, "default") {
			assert.NotContains(t, line, "*", "default should not show * when inactive")
		}
	}
}

// TestContextLs_envVarWarning verifies that a MODEL_RUNNER_HOST env var
// triggers a warning on stderr.
func TestContextLs_envVarWarning(t *testing.T) {
	setupContextTest(t)
	t.Setenv("MODEL_RUNNER_HOST", "http://override:9999")

	cmd := newContextLsCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)

	require.NoError(t, cmd.Execute())
	assert.Contains(t, errOut.String(), "MODEL_RUNNER_HOST")
	assert.Contains(t, errOut.String(), "override:9999")
}

// TestContextUse verifies that "context use" switches the active context.
func TestContextUse(t *testing.T) {
	store := setupContextTest(t)
	require.NoError(t, store.Create("myctx", modelctx.ContextConfig{
		Host: "http://localhost:12434",
	}))

	cmd := newContextUseCmd()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"myctx"})

	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "myctx")

	active, err := store.Active()
	require.NoError(t, err)
	assert.Equal(t, "myctx", active)
}

// TestContextUse_default verifies that "context use default" resets to
// auto-detection.
func TestContextUse_default(t *testing.T) {
	store := setupContextTest(t)
	require.NoError(t, store.Create("myctx", modelctx.ContextConfig{
		Host: "http://localhost:12434",
	}))
	require.NoError(t, store.SetActive("myctx"))

	cmd := newContextUseCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"default"})

	require.NoError(t, cmd.Execute())

	active, err := store.Active()
	require.NoError(t, err)
	assert.Equal(t, modelctx.DefaultContextName, active)
}

// TestContextUse_notFound verifies that "context use" returns an error for
// an unknown context name.
func TestContextUse_notFound(t *testing.T) {
	setupContextTest(t)

	cmd := newContextUseCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"nosuchctx"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestContextRm verifies that "context rm" removes a context.
func TestContextRm(t *testing.T) {
	store := setupContextTest(t)
	require.NoError(t, store.Create("myctx", modelctx.ContextConfig{
		Host: "http://localhost:12434",
	}))

	cmd := newContextRmCmd()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"myctx"})

	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "myctx")

	_, err := store.Get("myctx")
	require.Error(t, err) // should be gone
}

// TestContextRm_default verifies that "context rm default" returns an error.
func TestContextRm_default(t *testing.T) {
	setupContextTest(t)

	cmd := newContextRmCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"default"})

	err := cmd.Execute()
	require.Error(t, err)
}

// TestContextRm_active verifies that the active context cannot be removed.
func TestContextRm_active(t *testing.T) {
	store := setupContextTest(t)
	require.NoError(t, store.Create("myctx", modelctx.ContextConfig{
		Host: "http://localhost:12434",
	}))
	require.NoError(t, store.SetActive("myctx"))

	cmd := newContextRmCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"myctx"})

	err := cmd.Execute()
	require.Error(t, err)

	// Context must still exist.
	_, getErr := store.Get("myctx")
	require.NoError(t, getErr)
}

// TestContextRm_notFound verifies that removing an unknown context returns an
// error.
func TestContextRm_notFound(t *testing.T) {
	setupContextTest(t)

	cmd := newContextRmCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"nosuchctx"})

	err := cmd.Execute()
	require.Error(t, err)
}

// TestContextInspect verifies that "context inspect" outputs valid JSON
// containing the stored host.
func TestContextInspect(t *testing.T) {
	store := setupContextTest(t)
	require.NoError(t, store.Create("myctx", modelctx.ContextConfig{
		Host:        "http://192.168.1.100:12434",
		Description: "lab box",
	}))

	cmd := newContextInspectCmd()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"myctx"})

	require.NoError(t, cmd.Execute())

	var results []map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &results))
	require.Len(t, results, 1)
	assert.Equal(t, "myctx", results[0]["name"])
	assert.Equal(t, "http://192.168.1.100:12434", results[0]["host"])
	assert.Equal(t, "lab box", results[0]["description"])
}

// TestContextInspect_default verifies that "context inspect default" returns
// a synthetic JSON entry.
func TestContextInspect_default(t *testing.T) {
	setupContextTest(t)

	cmd := newContextInspectCmd()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"default"})

	require.NoError(t, cmd.Execute())

	var results []map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &results))
	require.Len(t, results, 1)
	assert.Equal(t, "default", results[0]["name"])
}

// TestContextInspect_notFound verifies that inspecting an unknown context
// returns an error.
func TestContextInspect_notFound(t *testing.T) {
	setupContextTest(t)

	cmd := newContextInspectCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"nosuchctx"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
