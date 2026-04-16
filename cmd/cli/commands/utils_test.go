package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/docker/model-runner/cmd/cli/desktop"
	mockdesktop "github.com/docker/model-runner/cmd/cli/mocks"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends/vllm"
	"github.com/docker/model-runner/pkg/inference/scheduling"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestStripDefaultsFromModelName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "ai prefix and latest tag",
			input:    "ai/gemma3:latest",
			expected: "gemma3",
		},
		{
			name:     "ai prefix with custom tag",
			input:    "ai/gemma3:v1",
			expected: "gemma3:v1",
		},
		{
			name:     "custom org with latest tag",
			input:    "myorg/gemma3:latest",
			expected: "myorg/gemma3",
		},
		{
			name:     "simple model name with latest",
			input:    "gemma3:latest",
			expected: "gemma3",
		},
		{
			name:     "simple model name without tag",
			input:    "gemma3",
			expected: "gemma3",
		},
		{
			name:     "ai prefix without tag",
			input:    "ai/gemma3",
			expected: "gemma3",
		},
		{
			name:     "huggingface model with latest",
			input:    "hf.co/bartowski/model:latest",
			expected: "hf.co/bartowski/model",
		},
		{
			name:     "huggingface model with custom tag",
			input:    "hf.co/bartowski/model:Q4_K_S",
			expected: "hf.co/bartowski/model:Q4_K_S",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "docker.io registry with ai prefix and latest tag",
			input:    "docker.io/ai/gemma3:latest",
			expected: "gemma3",
		},
		{
			name:     "index.docker.io registry with ai prefix and latest tag",
			input:    "index.docker.io/ai/gemma3:latest",
			expected: "gemma3",
		},
		{
			name:     "docker.io registry with ai prefix and custom tag",
			input:    "docker.io/ai/gemma3:v1",
			expected: "gemma3:v1",
		},
		{
			name:     "docker.io registry with custom org and latest tag",
			input:    "docker.io/myorg/gemma3:latest",
			expected: "myorg/gemma3",
		},
		{
			name:     "index.docker.io registry with custom org and latest tag",
			input:    "index.docker.io/myorg/gemma3:latest",
			expected: "myorg/gemma3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripDefaultsFromModelName(tt.input)
			if result != tt.expected {
				t.Errorf("stripDefaultsFromModelName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestHandleClientErrorFormat verifies that the error format follows the expected pattern.
func TestHandleClientErrorFormat(t *testing.T) {
	t.Run("error format is message: original error", func(t *testing.T) {
		originalErr := fmt.Errorf("network timeout")
		message := "Failed to fetch data"

		result := handleClientError(originalErr, message)

		expected := fmt.Errorf("%s: %w", message, originalErr).Error()
		if result.Error() != expected {
			t.Errorf("Error format mismatch.\nExpected: %q\nGot: %q", expected, result.Error())
		}

		if !errors.Is(result, originalErr) {
			t.Error("Error wrapping is not preserved - errors.Is() check failed")
		}
	})
}

func setupDesktopClientStatusMock(t *testing.T, ctrl *gomock.Controller, backendStatus map[string]string) {
	t.Helper()

	client := mockdesktop.NewMockDockerHttpClient(ctrl)
	modelRunner = desktop.NewContextForMock(client)
	desktopClient = desktop.New(modelRunner)

	statusJSON, err := json.Marshal(backendStatus)
	require.NoError(t, err)

	expectedModelsURL := modelRunner.URL(inference.ModelsPrefix)
	expectedStatusURL := modelRunner.URL(inference.InferencePrefix + "/status")
	expectedUserAgent := "docker-model-cli/" + desktop.Version

	client.EXPECT().Do(gomock.Cond(func(req any) bool {
		r, ok := req.(*http.Request)
		return ok && r.URL.String() == expectedModelsURL && r.Header.Get("User-Agent") == expectedUserAgent
	})).Return(&http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}"))}, nil)

	client.EXPECT().Do(gomock.Cond(func(req any) bool {
		r, ok := req.(*http.Request)
		return ok && r.URL.String() == expectedStatusURL && r.Header.Get("User-Agent") == expectedUserAgent
	})).Return(&http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(statusJSON))}, nil)
}

func TestCheckBackendInstalled(t *testing.T) {
	t.Run("running status string is treated as installed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		setupDesktopClientStatusMock(t, ctrl, map[string]string{"vllm": "running vllm latest-cuda"})

		installed, err := CheckBackendInstalled(vllm.Name)
		require.NoError(t, err)
		require.True(t, installed)
	})

	t.Run("not running status is treated as missing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		setupDesktopClientStatusMock(t, ctrl, map[string]string{"vllm": "not running"})

		installed, err := CheckBackendInstalled(vllm.Name)
		require.NoError(t, err)
		require.False(t, installed)
	})

	t.Run("error status is treated as missing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		setupDesktopClientStatusMock(t, ctrl, map[string]string{"vllm": "error failed to start"})

		installed, err := CheckBackendInstalled(vllm.Name)
		require.NoError(t, err)
		require.False(t, installed)
	})
}

func TestPromptInstallBackend(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.SetIn(strings.NewReader("yes\n"))
	out := new(bytes.Buffer)
	cmd.SetOut(out)

	confirmed, err := PromptInstallBackend(vllm.Name, cmd)
	require.NoError(t, err)
	require.True(t, confirmed)
	require.Contains(t, out.String(), "Backend \"vllm\" is not installed")
}

func TestEnsureBackendAvailableCancelled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	setupDesktopClientStatusMock(t, ctrl, map[string]string{"vllm": "not running"})

	cmd := &cobra.Command{Use: "test"}
	cmd.SetIn(strings.NewReader("n\n"))
	out := new(bytes.Buffer)
	cmd.SetOut(out)

	err := EnsureBackendAvailable(vllm.Name, cmd)
	require.Error(t, err)
	require.ErrorIs(t, err, errBackendInstallationCancelled)
	require.Contains(t, out.String(), "docker model install-runner --backend vllm")
}

func TestResolveRequiredBackend(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mockdesktop.NewMockDockerHttpClient(ctrl)
	modelRunner = desktop.NewContextForMock(client)
	desktopClient = desktop.New(modelRunner)

	model := "ai/functiongemma-vllm:270M"
	selection := scheduling.ModelBackendSelection{Backend: vllm.Name, Installed: false}
	body, err := json.Marshal(selection)
	require.NoError(t, err)

	expectedResolveURL := modelRunner.URL(inference.ModelsPrefix + "/backend?model=" + url.QueryEscape(model))
	expectedUserAgent := "docker-model-cli/" + desktop.Version

	client.EXPECT().Do(gomock.Cond(func(req any) bool {
		r, ok := req.(*http.Request)
		return ok && r.URL.String() == expectedResolveURL && r.Header.Get("User-Agent") == expectedUserAgent
	})).Return(&http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}, nil)

	backend, err := ResolveRequiredBackend(model)
	require.NoError(t, err)
	require.Equal(t, vllm.Name, backend)
}
