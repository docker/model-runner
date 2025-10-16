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

	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/context/docker"
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
		if strings.Contains(serverInfo.KernelVersion, "-microsoft-standard-WSL2") {
			// We can use Docker Desktop from within a WSL2 integrated distro.
			// https://github.com/search?q=repo%3Amicrosoft%2FWSL2-Linux-Kernel+path%3A%2F%5Earch%5C%2F.*%5C%2Fconfigs%5C%2Fconfig-wsl%2F+CONFIG_LOCALVERSION&type=code
			return serverInfo.OperatingSystem == "Docker Desktop"
		}
		return false
	}

	// Enforce that we're on macOS or Windows, just in case someone is running
	// a Docker client on (say) BSD.
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		return false
	}

	// docker run -it --rm --privileged --pid=host justincormack/nsenter1 /bin/sh -c 'cat /etc/os-release'
	return serverInfo.OperatingSystem == "Docker Desktop"
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
	// externalOpenAI indicates if this context points to an external OpenAI-compatible endpoint
	externalOpenAI bool
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

// DetectContext determines the current Docker Model Runner context.
func DetectContext(ctx context.Context, cli *command.DockerCli) (*ModelRunnerContext, error) {
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
	}

	// Compute the URL prefix based on the associated engine kind.
	var rawURLPrefix string
	if kind == types.ModelRunnerEngineKindMoby {
		rawURLPrefix = "http://localhost:" + strconv.Itoa(standalone.DefaultControllerPortMoby)
	} else if kind == types.ModelRunnerEngineKindCloud {
		rawURLPrefix = "http://localhost:" + strconv.Itoa(standalone.DefaultControllerPortCloud)
	} else if kind == types.ModelRunnerEngineKindMobyManual {
		rawURLPrefix = modelRunnerHost
	} else { // ModelRunnerEngineKindDesktop
		rawURLPrefix = "http://localhost" + inference.ExperimentalEndpointsPrefix
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

// NewContextWithHostPort creates a new ModelRunnerContext with a custom host and port.
func NewContextWithHostPort(cli *command.DockerCli, host string, port int) (*ModelRunnerContext, error) {
	if host == "" {
		host = "127.0.0.1"
	}
	if port == 0 {
		port = 12434
	}

	// Create URL prefix with custom host and port
	rawURLPrefix := fmt.Sprintf("http://%s:%d", host, port)
	urlPrefix, err := url.Parse(rawURLPrefix)
	if err != nil {
		return nil, fmt.Errorf("invalid model runner URL (%s): %w", rawURLPrefix, err)
	}

	// Use HTTP default client for custom host/port
	client := http.DefaultClient

	if userAgent := os.Getenv("USER_AGENT"); userAgent != "" {
		setUserAgent(client, userAgent)
	}

	return &ModelRunnerContext{
		kind:      types.ModelRunnerEngineKindMobyManual,
		urlPrefix: urlPrefix,
		client:    client,
	}, nil
}

// NewContextWithURL creates a new ModelRunnerContext with a custom URL.
// If external is true, this is an external OpenAI-compatible endpoint.
func NewContextWithURL(cli *command.DockerCli, rawURL string) (*ModelRunnerContext, error) {
	return NewContextWithURLExternal(cli, rawURL, false)
}

// NewContextWithURLExternal creates a new ModelRunnerContext with a custom URL.
// The external parameter indicates if this is an external OpenAI-compatible endpoint.
func NewContextWithURLExternal(cli *command.DockerCli, rawURL string, external bool) (*ModelRunnerContext, error) {
	urlPrefix, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid model runner URL (%s): %w", rawURL, err)
	}

	// Use HTTP default client for custom URL
	client := http.DefaultClient

	if userAgent := os.Getenv("USER_AGENT"); userAgent != "" {
		setUserAgent(client, userAgent)
	}

	return &ModelRunnerContext{
		kind:           types.ModelRunnerEngineKindMobyManual,
		urlPrefix:      urlPrefix,
		client:         client,
		externalOpenAI: external,
	}, nil
}

// IsExternalOpenAI returns true if this context points to an external OpenAI-compatible endpoint.
func (c *ModelRunnerContext) IsExternalOpenAI() bool {
	return c.externalOpenAI
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
