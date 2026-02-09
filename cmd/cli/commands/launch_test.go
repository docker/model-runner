package commands

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/cmd/cli/pkg/types"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

const (
	testBaseURL       = "http://example.com"
	testImage         = "test/image:latest"
	testHostPort      = 3000
	testContainerPort = 8080
)

func testContainerApp(envFn func(string) []string) containerApp {
	return containerApp{
		defaultImage:    testImage,
		defaultHostPort: testHostPort,
		containerPort:   testContainerPort,
		envFn:           envFn,
	}
}

func newTestCmd(buf *bytes.Buffer) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetOut(buf)
	return cmd
}

func TestSupportedAppsContainsAllRegistered(t *testing.T) {
	for name := range containerApps {
		require.Contains(t, supportedApps, name, "containerApps entry %q missing from supportedApps", name)
	}
	for name := range hostApps {
		require.Contains(t, supportedApps, name, "hostApps entry %q missing from supportedApps", name)
	}
	require.Equal(t, len(containerApps)+len(hostApps), len(supportedApps))
}

func TestResolveBaseEndpointsDesktop(t *testing.T) {
	expectedHost := "http://localhost" + inference.ExperimentalEndpointsPrefix
	ctx, err := desktop.NewContextForTest(
		expectedHost,
		nil,
		types.ModelRunnerEngineKindDesktop,
	)
	require.NoError(t, err)
	modelRunner = ctx

	ep, err := resolveBaseEndpoints(nil)
	require.NoError(t, err)
	require.Equal(t, "http://model-runner.docker.internal", ep.container)
	require.Equal(t, expectedHost, ep.host)
}

func TestResolveBaseEndpointsMobyManual(t *testing.T) {
	hostURL := "http://localhost:8080"
	ctx, err := desktop.NewContextForTest(
		hostURL,
		nil,
		types.ModelRunnerEngineKindMobyManual,
	)
	require.NoError(t, err)
	modelRunner = ctx

	ep, err := resolveBaseEndpoints(nil)
	require.NoError(t, err)
	require.Equal(t, "http://host.docker.internal:8080", ep.container)
	require.Equal(t, hostURL, ep.host)
}

func TestResolveBaseEndpointsCloud(t *testing.T) {
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
	ep, err := resolveBaseEndpoints(runner)
	require.NoError(t, err)
	require.Equal(t, "http://172.17.0.1:12435", ep.container)
	require.Equal(t, "http://127.0.0.1:12435", ep.host)
}

func TestResolveBaseEndpointsMoby(t *testing.T) {
	ctx, err := desktop.NewContextForTest(
		"http://localhost:12434",
		nil,
		types.ModelRunnerEngineKindMoby,
	)
	require.NoError(t, err)
	modelRunner = ctx

	runner := &standaloneRunner{
		gatewayIP:   "172.17.0.1",
		gatewayPort: 12434,
	}
	ep, err := resolveBaseEndpoints(runner)
	require.NoError(t, err)
	require.Equal(t, "http://172.17.0.1:12434", ep.container)
	require.Equal(t, "http://127.0.0.1:12434", ep.host)
}

func TestUnableToResolveBaseEndpointsCloud(t *testing.T) {
	ctx, err := desktop.NewContextForTest(
		"http://localhost:12435",
		nil,
		types.ModelRunnerEngineKindCloud,
	)
	require.NoError(t, err)
	modelRunner = ctx

	for _, tc := range []struct {
		name   string
		runner *standaloneRunner
	}{
		{"nil runner", nil},
		{"empty gateway and hostPort", &standaloneRunner{gatewayIP: "", gatewayPort: 0, hostPort: 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveBaseEndpoints(tc.runner)
			require.Error(t, err)
			require.Contains(t, err.Error(), "unable to determine standalone runner endpoint")
		})
	}
}

func TestResolveBaseEndpointsHostPortFallback(t *testing.T) {
	ctx, err := desktop.NewContextForTest(
		"http://localhost:12434",
		nil,
		types.ModelRunnerEngineKindMoby,
	)
	require.NoError(t, err)
	modelRunner = ctx

	runner := &standaloneRunner{hostPort: 12434}
	ep, err := resolveBaseEndpoints(runner)
	require.NoError(t, err)
	require.Equal(t, "http://host.docker.internal:12434", ep.container)
	require.Equal(t, "http://127.0.0.1:12434", ep.host)
}

func TestLaunchContainerAppDryRun(t *testing.T) {
	ca := testContainerApp(openaiEnv(openaiPathSuffix))
	buf := new(bytes.Buffer)
	cmd := newTestCmd(buf)

	err := launchContainerApp(cmd, ca, testBaseURL, "", 0, false, nil, true)
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "Would run: docker")
	require.Contains(t, output, "run --rm")
	require.Contains(t, output, fmt.Sprintf("-p %d:%d", testHostPort, testContainerPort))
	require.Contains(t, output, testImage)
	require.Contains(t, output, "OPENAI_API_BASE="+testBaseURL+"/engines/v1")
}

func TestLaunchContainerAppOverrides(t *testing.T) {
	ca := testContainerApp(openaiEnv(openaiPathSuffix))
	overrideImage := "custom/image:v2"
	overridePort := 9999
	buf := new(bytes.Buffer)
	cmd := newTestCmd(buf)

	err := launchContainerApp(cmd, ca, testBaseURL, overrideImage, overridePort, false, nil, true)
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, overrideImage)
	require.NotContains(t, output, testImage)
	require.Contains(t, output, fmt.Sprintf("-p %d:%d", overridePort, testContainerPort))
}

func TestLaunchContainerAppDetach(t *testing.T) {
	ca := testContainerApp(openaiEnv(openaiPathSuffix))
	buf := new(bytes.Buffer)
	cmd := newTestCmd(buf)

	err := launchContainerApp(cmd, ca, testBaseURL, "", 0, true, nil, true)
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "run --rm -d")
}

func TestLaunchContainerAppUsesEnvFn(t *testing.T) {
	customEnv := func(baseURL string) []string {
		return []string{"CUSTOM_URL=" + baseURL + "/custom"}
	}
	ca := testContainerApp(customEnv)
	buf := new(bytes.Buffer)
	cmd := newTestCmd(buf)

	err := launchContainerApp(cmd, ca, testBaseURL, "", 0, false, nil, true)
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "CUSTOM_URL="+testBaseURL+"/custom")
	require.NotContains(t, output, "OPENAI_API_BASE")
}

func TestLaunchContainerAppNilEnvFn(t *testing.T) {
	ca := testContainerApp(nil)
	buf := new(bytes.Buffer)
	cmd := newTestCmd(buf)

	err := launchContainerApp(cmd, ca, testBaseURL, "", 0, false, nil, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "container app requires envFn to be set")
}

func TestLaunchHostAppDryRunOpenai(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := newTestCmd(buf)

	cli := hostApp{envFn: openaiEnv(openaiPathSuffix)}
	// Use "ls" as a bin that exists in PATH
	err := launchHostApp(cmd, "ls", testBaseURL, cli, nil, true)
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "Would run: ls")
	require.Contains(t, output, "OPENAI_API_BASE="+testBaseURL+"/engines/v1")
	require.Contains(t, output, "OPENAI_BASE_URL="+testBaseURL+"/engines/v1")
	require.Contains(t, output, "OPENAI_API_KEY="+dummyAPIKey)
}

func TestLaunchHostAppDryRunCodex(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := newTestCmd(buf)

	cli := hostApp{envFn: openaiEnv("/v1")}
	err := launchHostApp(cmd, "ls", testBaseURL, cli, nil, true)
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "Would run: ls")
	require.Contains(t, output, "OPENAI_BASE_URL="+testBaseURL+"/v1")
	require.Contains(t, output, "OPENAI_API_KEY="+dummyAPIKey)
	require.NotContains(t, output, "/engines/v1")
}

func TestLaunchHostAppDryRunWithArgs(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := newTestCmd(buf)

	cli := hostApp{envFn: openaiEnv(openaiPathSuffix)}
	err := launchHostApp(cmd, "ls", testBaseURL, cli, []string{"-m", "ai/qwen3"}, true)
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "Would run: ls -m ai/qwen3")
}

func TestLaunchHostAppDryRunAnthropic(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := newTestCmd(buf)

	cli := hostApp{envFn: anthropicEnv}
	err := launchHostApp(cmd, "ls", testBaseURL, cli, nil, true)
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "Would run: ls")
	require.Contains(t, output, "ANTHROPIC_BASE_URL="+testBaseURL+"/anthropic")
	require.Contains(t, output, "ANTHROPIC_API_KEY="+dummyAPIKey)
	require.NotContains(t, output, "OPENAI_")
}

func TestLaunchHostAppNotFound(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := &cobra.Command{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	cli := hostApp{envFn: openaiEnv(openaiPathSuffix)}
	err := launchHostApp(cmd, "nonexistent-binary-xyz", testBaseURL, cli, nil, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")

	errOutput := stderr.String()
	require.Contains(t, errOutput, "not found in PATH")
	require.Contains(t, errOutput, "Configure your app to use:")
}

func TestLaunchHostAppNotFoundNilEnvFn(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := &cobra.Command{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	cli := hostApp{envFn: nil}
	err := launchHostApp(cmd, "nonexistent-binary-xyz", testBaseURL, cli, nil, false)
	require.Error(t, err)

	errOutput := stderr.String()
	require.Contains(t, errOutput, "not found in PATH")
	require.NotContains(t, errOutput, "Configure your app to use:")
}

func TestLaunchUnconfigurableHostAppDryRun(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := newTestCmd(buf)

	cli := hostApp{configInstructions: openclawConfigInstructions}
	err := launchUnconfigurableHostApp(cmd, "openclaw", testBaseURL, cli, nil, true)
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "Configure openclaw to use Docker Model Runner:")
	require.Contains(t, output, "Base URL: "+testBaseURL+"/engines/v1")
	require.Contains(t, output, "API type: openai-completions")
	require.Contains(t, output, "API key:  "+dummyAPIKey)
	require.Contains(t, output, "openclaw config set models.providers.docker-model-runner.baseUrl")
}

func TestNewLaunchCmdFlags(t *testing.T) {
	cmd := newLaunchCmd()

	require.NotNil(t, cmd.Flags().Lookup("port"))
	require.NotNil(t, cmd.Flags().Lookup("image"))
	require.NotNil(t, cmd.Flags().Lookup("detach"))
	require.NotNil(t, cmd.Flags().Lookup("dry-run"))
}

func TestNewLaunchCmdValidArgs(t *testing.T) {
	cmd := newLaunchCmd()
	require.Equal(t, supportedApps, cmd.ValidArgs)
}

func TestNewLaunchCmdRequiresAtLeastOneArg(t *testing.T) {
	cmd := newLaunchCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()

	require.Error(t, err)
	require.Contains(t, err.Error(), "requires at least 1 arg")
}

func TestNewLaunchCmdDispatchContainerApp(t *testing.T) {
	ctx, err := desktop.NewContextForTest(
		"http://localhost"+inference.ExperimentalEndpointsPrefix,
		nil,
		types.ModelRunnerEngineKindDesktop,
	)
	require.NoError(t, err)
	modelRunner = ctx

	buf := new(bytes.Buffer)
	cmd := newLaunchCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"openwebui", "--dry-run"})

	err = cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "Would run: docker")
	require.Contains(t, output, "ghcr.io/open-webui/open-webui:latest")
}

func TestNewLaunchCmdDispatchHostApp(t *testing.T) {
	ctx, err := desktop.NewContextForTest(
		"http://localhost"+inference.ExperimentalEndpointsPrefix,
		nil,
		types.ModelRunnerEngineKindDesktop,
	)
	require.NoError(t, err)
	modelRunner = ctx

	buf := new(bytes.Buffer)
	cmd := newLaunchCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"openclaw", "--dry-run"})

	err = cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	require.Contains(t, output, "Configure openclaw to use Docker Model Runner:")
}

func TestNewLaunchCmdDispatchUnsupportedApp(t *testing.T) {
	ctx, err := desktop.NewContextForTest(
		"http://localhost"+inference.ExperimentalEndpointsPrefix,
		nil,
		types.ModelRunnerEngineKindDesktop,
	)
	require.NoError(t, err)
	modelRunner = ctx

	buf := new(bytes.Buffer)
	cmd := newLaunchCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"bogus"})

	err = cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported app")
}
