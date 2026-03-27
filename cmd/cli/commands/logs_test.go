package commands

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/cmd/cli/pkg/types"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestMergedLogEqualTimestamps(t *testing.T) {
	f1 := filepath.Join(t.TempDir(), "a.log")
	if err := os.WriteFile(f1, []byte("[2026-03-09T12:00:00.000000000Z] line a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	f2 := filepath.Join(t.TempDir(), "b.log")
	if err := os.WriteFile(f2, []byte("[2026-03-09T12:00:00.000000000Z] line b\n"), 0644); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- printMergedLog(io.Discard, f1, f2) }()

	select {
	case <-done:
		// ok
	case <-time.After(3 * time.Second):
		t.Fatal("printMergedLog hung — equal timestamp deadlock")
	}
}

func TestMergedLogInterleavedTimestamps(t *testing.T) {
	f1 := filepath.Join(t.TempDir(), "a.log")
	if err := os.WriteFile(f1, []byte(strings.Join([]string{
		"[2026-03-09T12:00:00.000000000Z] a1",
		"[2026-03-09T12:00:02.000000000Z] a2",
		"[2026-03-09T12:00:04.000000000Z] a3",
	}, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	f2 := filepath.Join(t.TempDir(), "b.log")
	if err := os.WriteFile(f2, []byte(strings.Join([]string{
		"[2026-03-09T12:00:00.000000000Z] b1",
		"[2026-03-09T12:00:01.000000000Z] b2",
		"[2026-03-09T12:00:03.000000000Z] b3",
		"[2026-03-09T12:00:05.000000000Z] b4",
	}, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err := printMergedLog(&buf, f1, f2)
	if err != nil {
		t.Fatal(err)
	}

	got := strings.TrimSpace(buf.String())
	want := strings.Join([]string{
		"[2026-03-09T12:00:00.000000000Z] a1",
		"[2026-03-09T12:00:00.000000000Z] b1",
		"[2026-03-09T12:00:01.000000000Z] b2",
		"[2026-03-09T12:00:02.000000000Z] a2",
		"[2026-03-09T12:00:03.000000000Z] b3",
		"[2026-03-09T12:00:04.000000000Z] a3",
		"[2026-03-09T12:00:05.000000000Z] b4",
	}, "\n")

	if got != want {
		t.Errorf("wrong merge order:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestIsLocalLogsTargetURL(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
		want   bool
	}{
		{
			name:   "localhost",
			rawURL: "http://localhost:12434",
			want:   true,
		},
		{
			name:   "loopback IPv4",
			rawURL: "https://127.0.0.1:12434",
			want:   true,
		},
		{
			name:   "loopback IPv6",
			rawURL: "http://[::1]:12434",
			want:   true,
		},
		{
			name:   "Docker host alias",
			rawURL: "http://host.docker.internal:12434",
			want:   true,
		},
		{
			name:   "Model runner alias",
			rawURL: "http://model-runner.docker.internal",
			want:   true,
		},
		{
			name:   "Gateway alias",
			rawURL: "http://gateway.docker.internal",
			want:   true,
		},
		{
			name:   "remote hostname",
			rawURL: "http://example.com:12434",
			want:   false,
		},
		{
			name:   "invalid URL",
			rawURL: "://bad",
			want:   false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, isLocalLogsTargetURL(test.rawURL))
		})
	}
}

func TestShouldUseContainerLogs(t *testing.T) {
	tests := []struct {
		name              string
		engineKind        types.ModelRunnerEngineKind
		manualTargetLocal bool
		want              bool
	}{
		{
			name:       "Moby uses container logs",
			engineKind: types.ModelRunnerEngineKindMoby,
			want:       true,
		},
		{
			name:       "Cloud uses container logs",
			engineKind: types.ModelRunnerEngineKindCloud,
			want:       true,
		},
		{
			name:       "Desktop uses file logs",
			engineKind: types.ModelRunnerEngineKindDesktop,
			want:       false,
		},
		{
			name:              "Manual local uses container logs",
			engineKind:        types.ModelRunnerEngineKindMobyManual,
			manualTargetLocal: true,
			want:              true,
		},
		{
			name:              "Manual remote uses file logs",
			engineKind:        types.ModelRunnerEngineKindMobyManual,
			manualTargetLocal: false,
			want:              false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := shouldUseContainerLogs(
				test.engineKind,
				test.manualTargetLocal,
			)
			require.Equal(t, test.want, got)
		})
	}
}

func TestRunLogsForEnvManualLocalLinuxUsesContainerLogs(t *testing.T) {
	restoreModelRunner := setTestLogsModelRunner(
		t,
		"http://model-runner.docker.internal",
		types.ModelRunnerEngineKindMobyManual,
	)
	defer restoreModelRunner()

	client := &fakeLogsDockerClient{
		result: newMultiplexedLogStream(
			t,
			"stdout line\n",
			"stderr line\n",
		),
	}
	restoreSeams := setLogsTestSeams(
		t,
		func() (logsDockerClient, error) {
			return client, nil
		},
		func(ctx context.Context, dockerClient logsDockerClient) (string, error) {
			return "controller-id", nil
		},
	)
	defer restoreSeams()

	cmd, stdout, stderr := newTestLogsCommand()

	err := runLogsForEnv(cmd, false, false, "linux", false)
	require.NoError(t, err)
	require.Equal(t, "stdout line\n", stdout.String())
	require.Equal(t, "stderr line\n", stderr.String())
	require.Equal(t, 1, client.calls)
	require.Equal(t, "controller-id", client.containerID)
	require.True(t, client.options.ShowStdout)
	require.True(t, client.options.ShowStderr)
	require.False(t, client.options.Follow)
}

func TestRunLogsForEnvManualLocalFallsBackToFileLogs(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	writeDarwinLogFiles(
		t,
		homeDir,
		[]string{"[2026-03-09T12:00:00.000000000Z] service"},
		[]string{"[2026-03-09T12:00:01.000000000Z] runtime"},
	)

	restoreModelRunner := setTestLogsModelRunner(
		t,
		"http://model-runner.docker.internal",
		types.ModelRunnerEngineKindMobyManual,
	)
	defer restoreModelRunner()

	var clientFactoryCalled bool
	restoreSeams := setLogsTestSeams(
		t,
		func() (logsDockerClient, error) {
			clientFactoryCalled = true
			return nil, errors.New("docker unavailable")
		},
		nil,
	)
	defer restoreSeams()

	cmd, stdout, stderr := newTestLogsCommand()

	err := runLogsForEnv(cmd, false, false, "darwin", false)
	require.NoError(t, err)
	require.True(t, clientFactoryCalled)
	require.Equal(
		t,
		strings.Join([]string{
			"[2026-03-09T12:00:00.000000000Z] service",
			"[2026-03-09T12:00:01.000000000Z] runtime",
			"",
		}, "\n"),
		stdout.String(),
	)
	require.Empty(t, stderr.String())
}

func TestRunLogsForEnvManualLocalLinuxReturnsPreciseError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	restoreModelRunner := setTestLogsModelRunner(
		t,
		"http://model-runner.docker.internal",
		types.ModelRunnerEngineKindMobyManual,
	)
	defer restoreModelRunner()

	rootErr := errors.New("permission denied")
	restoreSeams := setLogsTestSeams(
		t,
		func() (logsDockerClient, error) {
			return nil, rootErr
		},
		nil,
	)
	defer restoreSeams()

	cmd, _, _ := newTestLogsCommand()

	err := runLogsForEnv(cmd, false, false, "linux", false)
	require.Error(t, err)
	require.ErrorIs(t, err, rootErr)
	require.ErrorContains(t, err, "MODEL_RUNNER_HOST")
	require.ErrorContains(t, err, "local Docker daemon")
	require.ErrorContains(t, err, "docker-model-runner container")
	require.NotContains(
		t,
		err.Error(),
		"only supported in standalone mode",
	)
}

func TestRunLogsForEnvMobyDoesNotFallBackToFiles(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	writeDarwinLogFiles(
		t,
		homeDir,
		[]string{"[2026-03-09T12:00:00.000000000Z] service"},
		[]string{"[2026-03-09T12:00:01.000000000Z] runtime"},
	)

	restoreModelRunner := setTestLogsModelRunner(
		t,
		"http://localhost:12434",
		types.ModelRunnerEngineKindMoby,
	)
	defer restoreModelRunner()

	rootErr := errors.New("docker unavailable")
	restoreSeams := setLogsTestSeams(
		t,
		func() (logsDockerClient, error) {
			return nil, rootErr
		},
		nil,
	)
	defer restoreSeams()

	cmd, stdout, stderr := newTestLogsCommand()

	err := runLogsForEnv(cmd, false, false, "darwin", false)
	require.Error(t, err)
	require.ErrorContains(t, err, "failed to create Docker client")
	require.ErrorIs(t, err, rootErr)
	require.Empty(t, stdout.String())
	require.Empty(t, stderr.String())
}

func TestRunLogsForEnvManualRemoteDoesNotTryContainerLogs(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	writeDarwinLogFiles(
		t,
		homeDir,
		[]string{"[2026-03-09T12:00:00.000000000Z] service"},
		[]string{"[2026-03-09T12:00:01.000000000Z] runtime"},
	)

	restoreModelRunner := setTestLogsModelRunner(
		t,
		"http://example.com:12434",
		types.ModelRunnerEngineKindMobyManual,
	)
	defer restoreModelRunner()

	var clientFactoryCalled bool
	restoreSeams := setLogsTestSeams(
		t,
		func() (logsDockerClient, error) {
			clientFactoryCalled = true
			return nil, nil
		},
		nil,
	)
	defer restoreSeams()

	cmd, stdout, stderr := newTestLogsCommand()

	err := runLogsForEnv(cmd, false, false, "darwin", false)
	require.NoError(t, err)
	require.False(t, clientFactoryCalled)
	require.Equal(
		t,
		strings.Join([]string{
			"[2026-03-09T12:00:00.000000000Z] service",
			"[2026-03-09T12:00:01.000000000Z] runtime",
			"",
		}, "\n"),
		stdout.String(),
	)
	require.Empty(t, stderr.String())
}

type fakeLogsDockerClient struct {
	result      client.ContainerLogsResult
	err         error
	calls       int
	containerID string
	options     client.ContainerLogsOptions
}

func (f *fakeLogsDockerClient) ContainerLogs(
	ctx context.Context,
	container string,
	options client.ContainerLogsOptions,
) (client.ContainerLogsResult, error) {
	f.calls++
	f.containerID = container
	f.options = options
	return f.result, f.err
}

func setLogsTestSeams(
	t *testing.T,
	factory func() (logsDockerClient, error),
	finder func(context.Context, logsDockerClient) (string, error),
) func() {
	t.Helper()

	originalFactory := logsDockerClientFactory
	originalFinder := logsControllerContainerFinder

	if factory != nil {
		logsDockerClientFactory = factory
	}
	if finder != nil {
		logsControllerContainerFinder = finder
	}

	return func() {
		logsDockerClientFactory = originalFactory
		logsControllerContainerFinder = originalFinder
	}
}

func setTestLogsModelRunner(
	t *testing.T,
	endpoint string,
	engineKind types.ModelRunnerEngineKind,
) func() {
	t.Helper()

	originalModelRunner := modelRunner
	ctx, err := desktop.NewContextForTest(endpoint, nil, engineKind)
	require.NoError(t, err)
	modelRunner = ctx

	return func() {
		modelRunner = originalModelRunner
	}
}

func newTestLogsCommand() (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return cmd, stdout, stderr
}

func writeDarwinLogFiles(
	t *testing.T,
	homeDir string,
	serviceLines []string,
	runtimeLines []string,
) {
	t.Helper()

	logDir := filepath.Join(
		homeDir,
		"Library/Containers/com.docker.docker/Data/log/host",
	)
	require.NoError(t, os.MkdirAll(logDir, 0755))

	serviceLog := filepath.Join(logDir, "inference.log")
	runtimeLog := filepath.Join(logDir, "inference-llama.cpp-server.log")

	require.NoError(
		t,
		os.WriteFile(
			serviceLog,
			[]byte(strings.Join(serviceLines, "\n")+"\n"),
			0644,
		),
	)
	require.NoError(
		t,
		os.WriteFile(
			runtimeLog,
			[]byte(strings.Join(runtimeLines, "\n")+"\n"),
			0644,
		),
	)
}

func newMultiplexedLogStream(
	t *testing.T,
	stdoutText string,
	stderrText string,
) client.ContainerLogsResult {
	t.Helper()

	var buf bytes.Buffer
	writeMultiplexedFrame(t, &buf, byte(stdcopy.Stdout), stdoutText)
	writeMultiplexedFrame(t, &buf, byte(stdcopy.Stderr), stderrText)

	return io.NopCloser(bytes.NewReader(buf.Bytes()))
}

func writeMultiplexedFrame(
	t *testing.T,
	w io.Writer,
	stream byte,
	payload string,
) {
	t.Helper()

	header := make([]byte, 8)
	header[0] = stream
	binary.BigEndian.PutUint32(header[4:], uint32(len(payload)))

	_, err := w.Write(header)
	require.NoError(t, err)

	_, err = io.WriteString(w, payload)
	require.NoError(t, err)
}
