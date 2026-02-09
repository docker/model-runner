package commands

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/docker/model-runner/cmd/cli/pkg/types"
	"github.com/spf13/cobra"
)

// openaiPathSuffix is the path appended to the base URL for OpenAI-compatible endpoints.
const openaiPathSuffix = "/engines/v1"

// dummyAPIKey is a placeholder API key for Docker Model Runner (which doesn't require auth).
const dummyAPIKey = "sk-docker-model-runner" //nolint:gosec // not a real credential

// engineEndpoints holds the resolved base URLs (without path) for both
// client locations.
type engineEndpoints struct {
	// base URL reachable from inside a Docker container
	// (e.g., http://model-runner.docker.internal).
	container string
	// base URL reachable from the host machine
	// (e.g., http://127.0.0.1:12434).
	host string
}

// containerApp describes an app that runs as a Docker container.
type containerApp struct {
	defaultImage    string
	defaultHostPort int
	containerPort   int
	envFn           func(baseURL string) []string
	extraDockerArgs []string // additional docker run args (e.g., volume mounts)
}

// containerApps are launched via "docker run --rm".
var containerApps = map[string]containerApp{
	"anythingllm": {
		defaultImage:    "mintplexlabs/anythingllm:latest",
		defaultHostPort: 3001,
		containerPort:   3001,
		envFn:           anythingllmEnv,
		extraDockerArgs: []string{"-v", "anythingllm_storage:/app/server/storage"},
	},
	"openwebui": {defaultImage: "ghcr.io/open-webui/open-webui:latest", defaultHostPort: 3000, containerPort: 8080, envFn: openaiEnv(openaiPathSuffix)},
}

// hostApp describes a native CLI app launched on the host.
type hostApp struct {
	envFn              func(baseURL string) []string
	configInstructions func(baseURL string) []string // for apps that need manual config
}

// hostApps are launched as native executables on the host.
var hostApps = map[string]hostApp{
	"opencode": {envFn: openaiEnv(openaiPathSuffix)},
	"codex":    {envFn: openaiEnv("/v1")},
	"claude":   {envFn: anthropicEnv},
	"openclaw": {configInstructions: openclawConfigInstructions},
}

// supportedApps is derived from the registries above.
var supportedApps = func() []string {
	apps := make([]string, 0, len(containerApps)+len(hostApps))
	for name := range containerApps {
		apps = append(apps, name)
	}
	for name := range hostApps {
		apps = append(apps, name)
	}
	sort.Strings(apps)
	return apps
}()

func newLaunchCmd() *cobra.Command {
	var (
		port   int
		image  string
		detach bool
		dryRun bool
	)
	c := &cobra.Command{
		Use:   "launch APP [-- APP_ARGS...]",
		Short: "Launch an app configured to use Docker Model Runner",
		Long: fmt.Sprintf(`Launch an app configured to use Docker Model Runner.

Supported apps: %s`, strings.Join(supportedApps, ", ")),
		Args:      requireMinArgs(1, "launch", "APP [-- APP_ARGS...]"),
		ValidArgs: supportedApps,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := strings.ToLower(args[0])
			appArgs := args[1:]

			runner, err := getStandaloneRunner(cmd.Context())
			if err != nil {
				return fmt.Errorf("unable to determine standalone runner endpoint: %w", err)
			}

			ep, err := resolveBaseEndpoints(runner)
			if err != nil {
				return err
			}

			if ca, ok := containerApps[app]; ok {
				return launchContainerApp(cmd, ca, ep.container, image, port, detach, appArgs, dryRun)
			}
			if cli, ok := hostApps[app]; ok {
				return launchHostApp(cmd, app, ep.host, cli, appArgs, dryRun)
			}
			return fmt.Errorf("unsupported app %q (supported: %s)", app, strings.Join(supportedApps, ", "))
		},
	}
	c.Flags().IntVar(&port, "port", 0, "Host port to expose (web UIs)")
	c.Flags().StringVar(&image, "image", "", "Override container image for containerized apps")
	c.Flags().BoolVar(&detach, "detach", false, "Run containerized app in background")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "Print what would be executed without running it")
	return c
}

// resolveBaseEndpoints resolves the base URLs (without path) for both
// container and host client locations.
func resolveBaseEndpoints(runner *standaloneRunner) (engineEndpoints, error) {
	const (
		localhost          = "127.0.0.1"
		hostDockerInternal = "host.docker.internal"
	)

	kind := modelRunner.EngineKind()
	switch kind {
	case types.ModelRunnerEngineKindDesktop:
		return engineEndpoints{
			container: "http://model-runner.docker.internal",
			host:      strings.TrimRight(modelRunner.URL(""), "/"),
		}, nil
	case types.ModelRunnerEngineKindMobyManual:
		ep := strings.TrimRight(modelRunner.URL(""), "/")
		containerEP := strings.NewReplacer(
			"localhost", hostDockerInternal,
			localhost, hostDockerInternal,
		).Replace(ep)
		return engineEndpoints{container: containerEP, host: ep}, nil
	case types.ModelRunnerEngineKindCloud, types.ModelRunnerEngineKindMoby:
		if runner == nil {
			return engineEndpoints{}, errors.New("unable to determine standalone runner endpoint")
		}
		if runner.gatewayIP != "" && runner.gatewayPort != 0 {
			port := fmt.Sprintf("%d", runner.gatewayPort)
			return engineEndpoints{
				container: "http://" + net.JoinHostPort(runner.gatewayIP, port),
				host:      "http://" + net.JoinHostPort(localhost, port),
			}, nil
		}
		if runner.hostPort != 0 {
			hostPort := fmt.Sprintf("%d", runner.hostPort)
			return engineEndpoints{
				container: "http://" + net.JoinHostPort(hostDockerInternal, hostPort),
				host:      "http://" + net.JoinHostPort(localhost, hostPort),
			}, nil
		}
		return engineEndpoints{}, errors.New("unable to determine standalone runner endpoint")
	default:
		return engineEndpoints{}, fmt.Errorf("unhandled engine kind: %v", kind)
	}
}

// launchContainerApp launches a container-based app via "docker run".
func launchContainerApp(cmd *cobra.Command, ca containerApp, baseURL string, imageOverride string, portOverride int, detach bool, appArgs []string, dryRun bool) error {
	img := imageOverride
	if img == "" {
		img = ca.defaultImage
	}
	hostPort := portOverride
	if hostPort == 0 {
		hostPort = ca.defaultHostPort
	}

	dockerArgs := []string{"run", "--rm"}
	if detach {
		dockerArgs = append(dockerArgs, "-d")
	}
	dockerArgs = append(dockerArgs,
		"-p", fmt.Sprintf("%d:%d", hostPort, ca.containerPort),
	)
	dockerArgs = append(dockerArgs, ca.extraDockerArgs...)
	if ca.envFn == nil {
		return fmt.Errorf("container app requires envFn to be set")
	}
	for _, e := range ca.envFn(baseURL) {
		dockerArgs = append(dockerArgs, "-e", e)
	}
	dockerArgs = append(dockerArgs, img)
	dockerArgs = append(dockerArgs, appArgs...)

	if dryRun {
		cmd.Printf("Would run: docker %s\n", strings.Join(dockerArgs, " "))
		return nil
	}

	return runExternal(cmd, nil, "docker", dockerArgs...)
}

// launchHostApp launches a native host app executable.
func launchHostApp(cmd *cobra.Command, bin string, baseURL string, cli hostApp, appArgs []string, dryRun bool) error {
	if !dryRun {
		if _, err := exec.LookPath(bin); err != nil {
			cmd.PrintErrf("%q executable not found in PATH.\n", bin)
			if cli.envFn != nil {
				cmd.PrintErrf("Configure your app to use:\n")
				for _, e := range cli.envFn(baseURL) {
					cmd.PrintErrf("  %s\n", e)
				}
			}
			return fmt.Errorf("%s not found; please install it and re-run", bin)
		}
	}

	if cli.envFn == nil {
		return launchUnconfigurableHostApp(cmd, bin, baseURL, cli, appArgs, dryRun)
	}

	env := cli.envFn(baseURL)
	if dryRun {
		cmd.Printf("Would run: %s %s\n", bin, strings.Join(appArgs, " "))
		for _, e := range env {
			cmd.Printf("  %s\n", e)
		}
		return nil
	}
	return runExternal(cmd, withEnv(env...), bin, appArgs...)
}

// launchUnconfigurableHostApp handles host apps that need manual config rather than env vars.
func launchUnconfigurableHostApp(cmd *cobra.Command, bin string, baseURL string, cli hostApp, appArgs []string, dryRun bool) error {
	enginesEP := baseURL + openaiPathSuffix
	cmd.Printf("Configure %s to use Docker Model Runner:\n", bin)
	cmd.Printf("  Base URL: %s\n", enginesEP)
	cmd.Printf("  API type: openai-completions\n")
	cmd.Printf("  API key:  %s\n", dummyAPIKey)

	if cli.configInstructions != nil {
		cmd.Printf("\nExample:\n")
		for _, line := range cli.configInstructions(baseURL) {
			cmd.Printf("  %s\n", line)
		}
	}
	if dryRun {
		cmd.Printf("Would run: %s %s\n", bin, strings.Join(appArgs, " "))
		return nil
	}
	return runExternal(cmd, nil, bin, appArgs...)
}

// openclawConfigInstructions returns configuration commands for openclaw.
func openclawConfigInstructions(baseURL string) []string {
	ep := baseURL + openaiPathSuffix
	return []string{
		fmt.Sprintf("openclaw config set models.providers.docker-model-runner.baseUrl %q", ep),
		"openclaw config set models.providers.docker-model-runner.api openai-completions",
		fmt.Sprintf("openclaw config set models.providers.docker-model-runner.apiKey %s", dummyAPIKey),
	}
}

// openaiEnv returns an env builder that sets OpenAI-compatible
// environment variables using the given path suffix.
func openaiEnv(suffix string) func(string) []string {
	return func(baseURL string) []string {
		ep := baseURL + suffix
		return []string{
			"OPENAI_API_BASE=" + ep,
			"OPENAI_BASE_URL=" + ep,
			"OPENAI_API_BASE_URL=" + ep,
			"OPENAI_API_KEY=" + dummyAPIKey,
			"OPEN_AI_KEY=" + dummyAPIKey, // AnythingLLM uses this
		}
	}
}

// anythingllmEnv returns environment variables for AnythingLLM with Docker Model Runner provider.
func anythingllmEnv(baseURL string) []string {
	return []string{
		"STORAGE_DIR=/app/server/storage",
		"LLM_PROVIDER=docker-model-runner",
		"DOCKER_MODEL_RUNNER_BASE_PATH=" + baseURL,
	}
}

// anthropicEnv returns Anthropic-compatible environment variables.
func anthropicEnv(baseURL string) []string {
	return []string{
		"ANTHROPIC_BASE_URL=" + baseURL + "/anthropic",
		"ANTHROPIC_API_KEY=" + dummyAPIKey,
	}
}

// withEnv returns the current process environment extended with extra vars.
func withEnv(extra ...string) []string {
	return append(os.Environ(), extra...)
}

// runExternal executes a program inheriting stdio.
// Security: prog and progArgs are either hardcoded values or user-provided
// arguments that the user explicitly intends to pass to the launched app.
func runExternal(cmd *cobra.Command, env []string, prog string, progArgs ...string) error {
	c := exec.Command(prog, progArgs...)
	c.Stdout = cmd.OutOrStdout()
	c.Stderr = cmd.ErrOrStderr()
	c.Stdin = os.Stdin
	if env != nil {
		c.Env = env
	}
	if err := c.Run(); err != nil {
		return fmt.Errorf("failed to run %s %s: %w", prog, strings.Join(progArgs, " "), err)
	}
	return nil
}
