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
	modeltls "github.com/docker/model-runner/pkg/tls"
	"github.com/moby/moby/client"
)

// isDesktopContext returns true if the CLI instance points to a Docker Desktop
// context and false otherwise.
func isDesktopContext(ctx context.Context, cli *command.DockerCli) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	serverInfo, _ := cli.Client().Info(ctx, client.InfoOptions{})

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
	return serverInfo.Info.OperatingSystem == "Docker Desktop"
}

func IsDesktopWSLContext(ctx context.Context, cli *command.DockerCli) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	serverInfo, _ := cli.Client().Info(ctx, client.InfoOptions{})

	return strings.Contains(serverInfo.Info.KernelVersion, "-microsoft-standard-WSL2") &&
		serverInfo.Info.OperatingSystem == "Docker Desktop"
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
	// openaiPathPrefix is the path prefix for OpenAI-compatible endpoints.
	// For internal Docker Model Runner, this is "/engines/v1".
	// For external OpenAI-compatible endpoints, this is empty (the URL already includes the version path).
	openaiPathPrefix string
	// useTLS indicates whether TLS is being used for connections.
	useTLS bool
	// tlsURLPrefix is the TLS URL prefix (if TLS is enabled).
	tlsURLPrefix *url.URL
	// tlsClient is the TLS-enabled HTTP client (if TLS is enabled).
	tlsClient DockerHttpClient
}

// NewContextForMock is a ModelRunnerContext constructor exposed only for the
// purposes of mock testing.
func NewContextForMock(client DockerHttpClient) *ModelRunnerContext {
	urlPrefix, err := url.Parse("http://localhost" + inference.ExperimentalEndpointsPrefix)
	if err != nil {
		panic("error occurred while parsing known-good URL")
	}
	return &ModelRunnerContext{
		kind:             types.ModelRunnerEngineKindDesktop,
		urlPrefix:        urlPrefix,
		client:           client,
		openaiPathPrefix: inference.InferencePrefix + "/v1",
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
		kind:             kind,
		urlPrefix:        urlPrefix,
		client:           client,
		openaiPathPrefix: inference.InferencePrefix + "/v1",
	}, nil
}

// NewContextForOpenAI creates a ModelRunnerContext for connecting to an external
// OpenAI-compatible API endpoint. This is used when the --openaiurl flag is specified.
func NewContextForOpenAI(endpoint string) (*ModelRunnerContext, error) {
	urlPrefix, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid OpenAI endpoint URL: %w", err)
	}

	return &ModelRunnerContext{
		kind:             types.ModelRunnerEngineKindMobyManual,
		urlPrefix:        urlPrefix,
		client:           http.DefaultClient,
		openaiPathPrefix: "", // Empty prefix for external OpenAI-compatible endpoints
	}, nil
}

// wakeUpCloudIfIdle checks if the Docker Cloud context is idle and wakes it up if needed.
func wakeUpCloudIfIdle(ctx context.Context, cli *command.DockerCli) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	info, err := cli.Client().Info(ctx, client.InfoOptions{})
	if err != nil {
		return fmt.Errorf("failed to get Docker info: %w", err)
	}

	// Check if the cloud.docker.run.engine label is set to "idle".
	isIdle := false
	for _, label := range info.Info.Labels {
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
	info, err = cli.Client().Info(ctx, client.InfoOptions{})
	if err != nil {
		return fmt.Errorf("failed to verify Docker Cloud wake-up: %w", err)
	}

	for _, label := range info.Info.Labels {
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

	// Check if TLS should be used
	useTLS := os.Getenv("MODEL_RUNNER_TLS") == "true"
	tlsSkipVerify := os.Getenv("MODEL_RUNNER_TLS_SKIP_VERIFY") == "true"
	tlsCACert := os.Getenv("MODEL_RUNNER_TLS_CA_CERT")

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
	var rawTLSURLPrefix string
	switch kind {
	case types.ModelRunnerEngineKindMoby:
		rawURLPrefix = "http://localhost:" + strconv.Itoa(standalone.DefaultControllerPortMoby)
		rawTLSURLPrefix = "https://localhost:" + strconv.Itoa(standalone.DefaultTLSPortMoby)
	case types.ModelRunnerEngineKindCloud:
		rawURLPrefix = "http://localhost:" + strconv.Itoa(standalone.DefaultControllerPortCloud)
		rawTLSURLPrefix = "https://localhost:" + strconv.Itoa(standalone.DefaultTLSPortCloud)
	case types.ModelRunnerEngineKindMobyManual:
		normalizedHost := modelRunnerHost

		// Ensure the manual host has a scheme.
		// Default to https when TLS is requested, otherwise http.
		if !strings.HasPrefix(normalizedHost, "http://") && !strings.HasPrefix(normalizedHost, "https://") {
			if useTLS {
				normalizedHost = "https://" + normalizedHost
			} else {
				normalizedHost = "http://" + normalizedHost
			}
		}

		rawURLPrefix = normalizedHost

		// Derive TLS URL from the normalized host, ensuring https when TLS is enabled.
		if useTLS {
			if strings.HasPrefix(normalizedHost, "http://") {
				rawTLSURLPrefix = "https://" + strings.TrimPrefix(normalizedHost, "http://")
			} else {
				rawTLSURLPrefix = normalizedHost
			}
		} else {
			rawTLSURLPrefix = normalizedHost
		}
	case types.ModelRunnerEngineKindDesktop:
		rawURLPrefix = "http://localhost" + inference.ExperimentalEndpointsPrefix
		rawTLSURLPrefix = rawURLPrefix // TLS not typically used with Desktop
		if IsDesktopWSLContext(ctx, cli) {
			dockerClient, err := DockerClientForContext(cli, cli.CurrentContext())
			if err != nil {
				return nil, fmt.Errorf("failed to create Docker client: %w", err)
			}

			// Check if a model runner container exists.
			containerID, _, _, err := standalone.FindControllerContainer(ctx, dockerClient)
			if err == nil && containerID != "" {
				rawURLPrefix = "http://localhost:" + strconv.Itoa(standalone.DefaultControllerPortMoby)
				rawTLSURLPrefix = "https://localhost:" + strconv.Itoa(standalone.DefaultTLSPortMoby)
				kind = types.ModelRunnerEngineKindMoby
			}
		}
	}
	urlPrefix, err := url.Parse(rawURLPrefix)
	if err != nil {
		return nil, fmt.Errorf("invalid model runner URL (%s): %w", rawURLPrefix, err)
	}

	var tlsURLPrefix *url.URL
	if useTLS {
		tlsURLPrefix, err = url.Parse(rawTLSURLPrefix)
		if err != nil {
			return nil, fmt.Errorf("invalid model runner TLS URL (%s): %w", rawTLSURLPrefix, err)
		}

		// Validate that TLS URL uses HTTPS when TLS is enabled
		if tlsURLPrefix.Scheme != "https" {
			return nil, fmt.Errorf("TLS requested but URL scheme is not HTTPS: %s", rawTLSURLPrefix)
		}
	}

	// Construct the HTTP client.
	var httpClient DockerHttpClient
	if kind == types.ModelRunnerEngineKindDesktop {
		dockerClient, err := DockerClientForContext(cli, cli.CurrentContext())
		if err != nil {
			return nil, fmt.Errorf("unable to create model runner client: %w", err)
		}
		httpClient = dockerClient.HTTPClient()
	} else {
		httpClient = http.DefaultClient
	}

	if userAgent := os.Getenv("USER_AGENT"); userAgent != "" {
		setUserAgent(httpClient, userAgent)
	}

	// Construct TLS client if TLS is enabled
	var tlsClient DockerHttpClient
	if useTLS {
		if kind == types.ModelRunnerEngineKindDesktop {
			// For Desktop context, if TLS is enabled, we should either fully support it or fail fast
			// Since Desktop context uses Docker client, we need to handle TLS differently
			// For now, we'll fail fast to make the behavior clear
			return nil, fmt.Errorf("TLS is not supported for Desktop contexts")
		}

		tlsConfig, err := modeltls.LoadClientTLSConfig(tlsCACert, tlsSkipVerify)
		if err != nil {
			return nil, fmt.Errorf("unable to load TLS configuration: %w", err)
		}

		tlsTransport := &http.Transport{
			TLSClientConfig: tlsConfig,
			Proxy:           http.ProxyFromEnvironment,
		}
		tlsClient = &http.Client{
			Transport: tlsTransport,
		}

		if userAgent := os.Getenv("USER_AGENT"); userAgent != "" {
			setUserAgent(tlsClient, userAgent)
		}
	}

	// Success.
	return &ModelRunnerContext{
		kind:             kind,
		urlPrefix:        urlPrefix,
		client:           httpClient,
		openaiPathPrefix: inference.InferencePrefix + "/v1",
		useTLS:           useTLS,
		tlsURLPrefix:     tlsURLPrefix,
		tlsClient:        tlsClient,
	}, nil
}

// EngineKind returns the Docker engine kind associated with the model runner.
func (c *ModelRunnerContext) EngineKind() types.ModelRunnerEngineKind {
	return c.kind
}

// URL constructs a URL string appropriate for the model runner.
// If TLS is enabled, returns the TLS URL.
func (c *ModelRunnerContext) URL(path string) string {
	prefix := c.urlPrefix
	if c.useTLS && c.tlsURLPrefix != nil {
		prefix = c.tlsURLPrefix
	}
	return c.buildURL(prefix, path)
}

// Client returns an HTTP client appropriate for accessing the model runner.
// If TLS is enabled, returns the TLS client.
func (c *ModelRunnerContext) Client() DockerHttpClient {
	if c.useTLS && c.tlsClient != nil {
		return c.tlsClient
	}
	return c.client
}

// UseTLS returns whether TLS is enabled for this context.
func (c *ModelRunnerContext) UseTLS() bool {
	return c.useTLS
}

// TLSURL constructs a TLS URL string for the model runner.
// Returns an empty string if TLS is not enabled.
func (c *ModelRunnerContext) TLSURL(path string) string {
	if c.tlsURLPrefix == nil {
		return ""
	}
	return c.buildURL(c.tlsURLPrefix, path)
}

// buildURL constructs a URL string from a prefix and path, handling query parameters.
func (c *ModelRunnerContext) buildURL(prefix *url.URL, path string) string {
	if prefix == nil {
		return ""
	}
	components := strings.Split(path, "?")
	result := prefix.JoinPath(components[0]).String()
	if len(components) > 1 {
		components[0] = result
		result = strings.Join(components, "?")
	}
	return result
}

// TLSClient returns the TLS HTTP client, or nil if TLS is not enabled.
func (c *ModelRunnerContext) TLSClient() DockerHttpClient {
	return c.tlsClient
}

// OpenAIPathPrefix returns the path prefix for OpenAI-compatible endpoints.
// For internal Docker Model Runner, this returns the inference prefix.
// For external OpenAI-compatible endpoints, this returns an empty string.
func (c *ModelRunnerContext) OpenAIPathPrefix() string {
	return c.openaiPathPrefix
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
