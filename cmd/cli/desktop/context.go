package desktop

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/context/docker"
	"github.com/docker/docker/api/types/container"
	clientpkg "github.com/docker/docker/client"
	"github.com/docker/model-runner/cmd/cli/pkg/standalone"
	"github.com/docker/model-runner/cmd/cli/pkg/types"
	"github.com/docker/model-runner/pkg/inference"
)

// isDesktopContext returns true if the CLI instance points to a Docker Desktop
// context and false otherwise.
func isDesktopContext(ctx context.Context, cli *command.DockerCli) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	serverInfo, _ := cli.Client().Info(ctx)

	// We don't currently support Docker Model Runner in Docker Desktop for
	// Linux, so we won't treat that as a Docker Desktop case (though it will
	// still work as a standard Moby or Cloud case, depending on configuration).
	if runtime.GOOS == "linux" {
		// We can use Docker Desktop from within a WSL2 integrated distro.
		// https://github.com/search?q=repo%3Amicrosoft%2FWSL2-Linux-Kernel+path%3A%2F%5Earch%5C%2F.*%5C%2Fconfigs%5C%2Fconfig-wsl%2F+CONFIG_LOCALVERSION&type=code
		return IsDesktopWSLContext(ctx, cli)
	}

	// Enforce that we're on macOS or Windows, just in case someone is running
	// a Docker client on (say) BSD.
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		return false
	}

	// docker run -it --rm --privileged --pid=host justincormack/nsenter1 /bin/sh -c 'cat /etc/os-release'
	return serverInfo.OperatingSystem == "Docker Desktop"
}

func IsDesktopWSLContext(ctx context.Context, cli *command.DockerCli) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	serverInfo, _ := cli.Client().Info(ctx)

	return strings.Contains(serverInfo.KernelVersion, "-microsoft-standard-WSL2") &&
		serverInfo.OperatingSystem == "Docker Desktop"
}

// isCloudContext returns true if the CLI instance points to a Docker Cloud
// context and false otherwise.
func isCloudContext(cli *command.DockerCli) bool {
	rawMetadata, err := cli.ContextStore().GetMetadata(cli.CurrentContext())
	if err != nil {
		return false
	}
	metadata, err := command.GetDockerContext(rawMetadata)
	if err != nil {
		return false
	}
	_, ok := metadata.AdditionalFields["cloud.docker.com"]
	return ok
}

// DockerClientForContext creates a Docker client for the specified context.
func DockerClientForContext(cli *command.DockerCli, name string) (*clientpkg.Client, error) {
	c, err := cli.ContextStore().GetMetadata(name)
	if err != nil {
		return nil, fmt.Errorf("unable to load context metadata: %w", err)
	}
	endpoint, err := docker.EndpointFromContext(c)
	if err != nil {
		return nil, fmt.Errorf("unable to determine context endpoint: %w", err)
	}
	return clientpkg.NewClientWithOpts(
		clientpkg.FromEnv,
		clientpkg.WithHost(endpoint.Host),
		clientpkg.WithAPIVersionNegotiation(),
	)
}

// ModelRunnerContext encodes the operational context of a Model CLI command and
// provides facilities for inspecting and interacting with the Model Runner.
type ModelRunnerContext struct {
	// kind stores the associated engine kind.
	kind types.ModelRunnerEngineKind
	// urlPrefix is the prefix URL for all requests.
	urlPrefix *url.URL
	// client is the model runner client.
	client DockerHttpClient
}

// NewContextForMock is a ModelRunnerContext constructor exposed only for the
// purposes of mock testing.
func NewContextForMock(client DockerHttpClient) *ModelRunnerContext {
	urlPrefix, err := url.Parse("http://localhost" + inference.ExperimentalEndpointsPrefix)
	if err != nil {
		panic("error occurred while parsing known-good URL")
	}
	return &ModelRunnerContext{
		kind:      types.ModelRunnerEngineKindDesktop,
		urlPrefix: urlPrefix,
		client:    client,
	}
}

// NewContextForTest creates a ModelRunnerContext for integration and mock testing
// with a custom URL endpoint and engine kind. This is intended for use in tests
// where the Model Runner endpoint is dynamically created (e.g., testcontainers).
func NewContextForTest(endpoint string, client DockerHttpClient, kind types.ModelRunnerEngineKind) (*ModelRunnerContext, error) {
	urlPrefix, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint URL: %w", err)
	}

	if client == nil {
		client = http.DefaultClient
	}

	return &ModelRunnerContext{
		kind:      kind,
		urlPrefix: urlPrefix,
		client:    client,
	}, nil
}

// wakeUpCloudIfIdle checks if the Docker Cloud context is idle and wakes it up if needed.
func wakeUpCloudIfIdle(ctx context.Context, cli *command.DockerCli) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	info, err := cli.Client().Info(ctx)
	if err != nil {
		return fmt.Errorf("failed to get Docker info: %w", err)
	}

	// Check if the cloud.docker.run.engine label is set to "idle".
	isIdle := false
	for _, label := range info.Labels {
		if label == "cloud.docker.run.engine=idle" {
			isIdle = true
			break
		}
	}
	if !isIdle {
		return nil
	}

	// Wake up Docker Cloud by triggering an empty ContainerCreate call.
	dockerClient, err := DockerClientForContext(cli, cli.CurrentContext())
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}

	// The call is expected to fail with a client error due to nil arguments, but it triggers
	// Docker Cloud to wake up from idle. Only return unexpected failures (network issues,
	// server errors) so they're logged as warnings.
	_, err = dockerClient.ContainerCreate(ctx, &container.Config{}, nil, nil, nil, "")
	if err != nil && !errdefs.IsInvalidArgument(err) {
		return fmt.Errorf("failed to wake up Docker Cloud: %w", err)
	}

	// Verify Docker Cloud is no longer idle.
	info, err = cli.Client().Info(ctx)
	if err != nil {
		return fmt.Errorf("failed to verify Docker Cloud wake-up: %w", err)
	}

	for _, label := range info.Labels {
		if label == "cloud.docker.run.engine=idle" {
			return fmt.Errorf("failed to wake up Docker Cloud from idle state")
		}
	}

	return nil
}

// DetectContext determines the current Docker Model Runner context.
func DetectContext(ctx context.Context, cli *command.DockerCli, printer standalone.StatusPrinter) (*ModelRunnerContext, error) {
	// Check for an explicit endpoint setting.
	modelRunnerHost := os.Getenv("MODEL_RUNNER_HOST")

	// Check if we're treating Docker Desktop as regular Moby. This is only for
	// testing purposes.
	treatDesktopAsMoby := os.Getenv("_MODEL_RUNNER_TREAT_DESKTOP_AS_MOBY") == "1"

	// Detect the associated engine type.
	kind := types.ModelRunnerEngineKindMoby
	if modelRunnerHost != "" {
		kind = types.ModelRunnerEngineKindMobyManual
	} else if isDesktopContext(ctx, cli) {
		kind = types.ModelRunnerEngineKindDesktop
		if treatDesktopAsMoby {
			kind = types.ModelRunnerEngineKindMoby
		}
	} else if isCloudContext(cli) {
		kind = types.ModelRunnerEngineKindCloud
		// Wake up Docker Cloud if it's idle.
		if err := wakeUpCloudIfIdle(ctx, cli); err != nil {
			// Log the error as a warning but don't fail - we'll try to use Docker Cloud anyway.
			// The downside is that the wrong docker/model-runner image might be automatically
			// pulled on docker install-runner because the runtime can't be properly verified.
			printer.Printf("Warning: %v\n", err)
		}
	}

	// Compute the URL prefix based on the associated engine kind.
	var rawURLPrefix string
	switch kind {
	case types.ModelRunnerEngineKindMoby:
		rawURLPrefix = "http://localhost:" + strconv.Itoa(standalone.DefaultControllerPortMoby)
	case types.ModelRunnerEngineKindCloud:
		rawURLPrefix = "http://localhost:" + strconv.Itoa(standalone.DefaultControllerPortCloud)
	case types.ModelRunnerEngineKindMobyManual:
		rawURLPrefix = modelRunnerHost
	case types.ModelRunnerEngineKindDesktop:
		rawURLPrefix = "http://localhost" + inference.ExperimentalEndpointsPrefix
		if IsDesktopWSLContext(ctx, cli) {
			dockerClient, err := DockerClientForContext(cli, cli.CurrentContext())
			if err != nil {
				return nil, fmt.Errorf("failed to create Docker client: %w", err)
			}

			// Check if a model runner container exists.
			containerID, _, _, err := standalone.FindControllerContainer(ctx, dockerClient)
			if err == nil && containerID != "" {
				rawURLPrefix = "http://localhost:" + strconv.Itoa(standalone.DefaultControllerPortMoby)
				kind = types.ModelRunnerEngineKindMoby
			}
		}
	}
	urlPrefix, err := url.Parse(rawURLPrefix)
	if err != nil {
		return nil, fmt.Errorf("invalid model runner URL (%s): %w", rawURLPrefix, err)
	}

	// Construct the HTTP client.
	var client DockerHttpClient
	if kind == types.ModelRunnerEngineKindDesktop {
		dockerClient, err := DockerClientForContext(cli, cli.CurrentContext())
		if err != nil {
			return nil, fmt.Errorf("unable to create model runner client: %w", err)
		}
		client = dockerClient.HTTPClient()
	} else {
		client = http.DefaultClient
	}

	if userAgent := os.Getenv("USER_AGENT"); userAgent != "" {
		setUserAgent(client, userAgent)
	}

	// Success.
	return &ModelRunnerContext{
		kind:      kind,
		urlPrefix: urlPrefix,
		client:    client,
	}, nil
}

// EngineKind returns the Docker engine kind associated with the model runner.
func (c *ModelRunnerContext) EngineKind() types.ModelRunnerEngineKind {
	return c.kind
}

// URL constructs a URL string appropriate for the model runner.
func (c *ModelRunnerContext) URL(path string) string {
	components := strings.Split(path, "?")
	result := c.urlPrefix.JoinPath(components[0]).String()
	if len(components) > 1 {
		components[0] = result
		result = strings.Join(components, "?")
	}
	return result
}

// Client returns an HTTP client appropriate for accessing the model runner.
func (c *ModelRunnerContext) Client() DockerHttpClient {
	return c.client
}

func setUserAgent(client DockerHttpClient, userAgent string) {
	if httpClient, ok := client.(*http.Client); ok {
		transport := httpClient.Transport
		if transport == nil {
			transport = http.DefaultTransport
		}

		httpClient.Transport = &userAgentTransport{
			userAgent: userAgent,
			transport: transport,
		}
	}
}

type userAgentTransport struct {
	userAgent string
	transport http.RoundTripper
}

func (u *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	reqClone := req.Clone(req.Context())

	existingUA := reqClone.UserAgent()

	var newUA string
	if existingUA != "" {
		newUA = existingUA + " " + u.userAgent
	} else {
		newUA = u.userAgent
	}

	reqClone.Header.Set("User-Agent", newUA)

	return u.transport.RoundTrip(reqClone)
}
