package commands

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/docker/cli/cli-plugins/hooks"
	"github.com/docker/model-runner/cmd/cli/desktop"
	mockdesktop "github.com/docker/model-runner/cmd/cli/mocks"
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

			req, err := http.NewRequest(http.MethodGet, modelRunner.URL(inference.ModelsPrefix), nil)
			require.NoError(t, err)
			req.Header.Set("User-Agent", "docker-model-cli/"+desktop.Version)
			client.EXPECT().Do(req).Return(test.doResponse, test.doErr)

			if test.doResponse != nil && test.doResponse.StatusCode == http.StatusOK {
				req, err = http.NewRequest(http.MethodGet, modelRunner.URL(inference.InferencePrefix+"/status"), nil)
				require.NoError(t, err)
				req.Header.Set("User-Agent", "docker-model-cli/"+desktop.Version)
				client.EXPECT().Do(req).Return(&http.Response{Body: mockBody}, test.doErr)
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

			err = cmd.Execute()
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
