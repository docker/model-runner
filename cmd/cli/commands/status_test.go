package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/docker/cli/cli-plugins/hooks"
	"github.com/docker/model-runner/cmd/cli/desktop"
	mockdesktop "github.com/docker/model-runner/cmd/cli/mocks"
	"github.com/docker/model-runner/cmd/cli/pkg/standalone"
	"github.com/docker/model-runner/cmd/cli/pkg/types"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBody := io.NopCloser(strings.NewReader(""))

	tests := []struct {
		name           string
		doResponse     *http.Response
		doErr          error
		expectExit     bool
		expectedErr    error
		expectedOutput string
	}{
		{
			name:           "running",
			doResponse:     &http.Response{StatusCode: http.StatusOK, Body: mockBody},
			doErr:          nil,
			expectExit:     false,
			expectedErr:    nil,
			expectedOutput: "Docker Model Runner is running\n",
		},
		{
			name:        "not running",
			doResponse:  &http.Response{StatusCode: http.StatusServiceUnavailable, Body: mockBody},
			doErr:       nil,
			expectExit:  true,
			expectedErr: nil,
			expectedOutput: func() string {
				buf := new(bytes.Buffer)
				fmt.Fprintln(buf, "Docker Model Runner is not running")
				hooks.PrintNextSteps(buf, []string{enableViaCLI, enableViaGUI})
				return buf.String()
			}(),
		},
		{
			name:       "request with error",
			doResponse: &http.Response{StatusCode: http.StatusInternalServerError, Body: mockBody},
			doErr:      nil,
			expectExit: false,
			expectedErr: handleClientError(
				fmt.Errorf("unexpected status code: %d", http.StatusInternalServerError),
				"Failed to get Docker Model Runner status",
			),
			expectedOutput: "",
		},
		{
			name:       "failed request",
			doResponse: nil,
			doErr:      fmt.Errorf("failed to make request"),
			expectExit: false,
			expectedErr: handleClientError(
				fmt.Errorf("error querying %s: %w", inference.ModelsPrefix, fmt.Errorf("failed to make request")),
				"Failed to get Docker Model Runner status",
			),
			expectedOutput: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := mockdesktop.NewMockDockerHttpClient(ctrl)
			modelRunner = desktop.NewContextForMock(client)
			desktopClient = desktop.New(modelRunner)

			// Match request by URL path and User-Agent header
			expectedURL := modelRunner.URL(inference.ModelsPrefix)
			expectedUserAgent := "docker-model-cli/" + desktop.Version
			client.EXPECT().Do(gomock.Cond(func(req any) bool {
				r, ok := req.(*http.Request)
				return ok && r.URL.String() == expectedURL && r.Header.Get("User-Agent") == expectedUserAgent
			})).Return(test.doResponse, test.doErr)

			if test.doResponse != nil && test.doResponse.StatusCode == http.StatusOK {
				expectedStatusURL := modelRunner.URL(inference.InferencePrefix + "/status")
				client.EXPECT().Do(gomock.Cond(func(req any) bool {
					r, ok := req.(*http.Request)
					return ok && r.URL.String() == expectedStatusURL && r.Header.Get("User-Agent") == expectedUserAgent
				})).Return(&http.Response{Body: mockBody}, test.doErr)
			}

			originalOsExit := osExit
			exitCalled := false
			osExit = func(code int) {
				exitCalled = true
				require.Equal(t, 1, code, "Expected exit code to be 1")
			}
			defer func() { osExit = originalOsExit }()

			cmd := newStatusCmd()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)

			err := cmd.Execute()
			if test.expectExit {
				require.True(t, exitCalled, "Expected os.Exit to be called")
			} else {
				require.False(t, exitCalled, "Did not expect os.Exit to be called")
			}
			if test.expectedErr != nil {
				require.Error(t, err)
				require.EqualError(t, err, test.expectedErr.Error())
			} else {
				require.NoError(t, err)
				require.True(t, strings.HasPrefix(buf.String(), test.expectedOutput))
			}
		})
	}
}

func TestJsonStatus(t *testing.T) {
	tests := []struct {
		name             string
		engineKind       types.ModelRunnerEngineKind
		urlPrefix        string
		expectedKind     string
		expectedEndpoint string
		expectedHostEnd  string
	}{
		{
			name:             "Docker Desktop",
			engineKind:       types.ModelRunnerEngineKindDesktop,
			urlPrefix:        "http://localhost" + inference.ExperimentalEndpointsPrefix,
			expectedKind:     "Docker Desktop",
			expectedEndpoint: "http://model-runner.docker.internal/v1/",
			expectedHostEnd:  "http://localhost" + inference.ExperimentalEndpointsPrefix + "/v1/",
		},
		{
			name:             "Docker Engine",
			engineKind:       types.ModelRunnerEngineKindMoby,
			urlPrefix:        "http://localhost:" + strconv.Itoa(standalone.DefaultControllerPortMoby),
			expectedKind:     "Docker Engine",
			expectedEndpoint: makeEndpoint("host.docker.internal", standalone.DefaultControllerPortMoby),
			expectedHostEnd:  makeEndpoint("127.0.0.1", standalone.DefaultControllerPortMoby),
		},
		{
			name:             "Docker Cloud",
			engineKind:       types.ModelRunnerEngineKindCloud,
			urlPrefix:        "http://localhost:" + strconv.Itoa(standalone.DefaultControllerPortCloud),
			expectedKind:     "Docker Cloud",
			expectedEndpoint: makeEndpoint("127.0.0.1", standalone.DefaultControllerPortCloud),
			expectedHostEnd:  makeEndpoint("127.0.0.1", standalone.DefaultControllerPortCloud),
		},
		{
			name:             "Docker Engine (Manual Install)",
			engineKind:       types.ModelRunnerEngineKindMobyManual,
			urlPrefix:        "http://localhost:8080",
			expectedKind:     "Docker Engine (Manual Install)",
			expectedEndpoint: "http://localhost:8080/v1/",
			expectedHostEnd:  "http://localhost:8080/v1/",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, err := desktop.NewContextForTest(test.urlPrefix, nil, test.engineKind)
			require.NoError(t, err)
			modelRunner = ctx

			var output string
			printer := desktop.NewSimplePrinter(func(msg string) {
				output = msg
			})
			status := desktop.Status{Running: true}
			backendStatus := map[string]string{"llama.cpp": "running"}

			// Cloud kind needs a runner for gateway IP/port
			var runner *standaloneRunner
			if test.engineKind == types.ModelRunnerEngineKindCloud {
				runner = &standaloneRunner{}
			}

			err = jsonStatus(printer, runner, status, backendStatus)
			require.NoError(t, err)

			var result map[string]any
			err = json.Unmarshal([]byte(output), &result)
			require.NoError(t, err)

			require.Equal(t, test.expectedKind, result["kind"])
			require.Equal(t, test.expectedEndpoint, result["endpoint"])
			require.Equal(t, test.expectedHostEnd, result["endpointHost"])
			require.Equal(t, true, result["running"])
		})
	}
}
