package openai

import (
	"context"
	"errors"
	"fmt"
	logpkg "log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/logging"
)

const (
	// Name is the backend name.
	Name = "openai"
	// openAIAPIBaseURL is the the base URL for the OpenAI API.
	openAIAPIBaseURL = "https://api.openai.com/v1/"
)

// openai is the OpenAI passthrough backend implementation.
type openai struct {
	// log is the associated logger.
	log logging.Logger
	// serverLog is the logger to use for proxy and request serving logs.
	serverLog logging.Logger
}

// New creates a new OpenAI passthrough backend.
func New(
	log logging.Logger,
	serverLog logging.Logger,
) (inference.Backend, error) {
	return &openai{
		log:       log,
		serverLog: serverLog,
	}, nil
}

// Name implements inference.Backend.Name.
func (*openai) Name() string {
	return Name
}

// Passthrough implements inference.Backend.Passthrough.
func (*openai) Passthrough() bool {
	return true
}

// Install implements inference.Backend.Install.
func (*openai) Install(_ context.Context, _ *http.Client) error {
	return nil
}

// stripPathToEndOfV1 strips a request URL path to remove any prefix up to the
// end of the first occurrence of "/v1". If no occurrence is found, then the
// path is returned unmodified.
func stripPathToEndOfV1(path string) string {
	index := strings.Index(path, "/v1")
	if index < 0 {
		return path
	}
	return path[index+3:]
}

// Run implements inference.Backend.Run.
func (o *openai) Run(ctx context.Context, socket, _ string, _ inference.BackendMode, _ *inference.BackendConfiguration) error {
	// Set up a reverse proxy to forward requests from the assigned Unix domain
	// socket to OpenAI's API endpoint.
	upstream, err := url.Parse(openAIAPIBaseURL)
	if err != nil {
		return fmt.Errorf("unable to parse OpenAI API URL: %w", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	nominalDirector := proxy.Director
	if nominalDirector == nil {
		return errors.New("invalid reverse proxy director created")
	}
	proxy.Director = func(r *http.Request) {
		// Remove any host specification - we want the URL host to be used.
		r.Host = ""
		r.Header.Del("Host")

		// The nominal director will concatenate URLs. It adjusts trailing and
		// leading slashes automatically, so we don't need to worry too much
		// there, but we do need to remove the overlapping portions of our API
		// endpoints.
		r.URL.Path = stripPathToEndOfV1(r.URL.Path)
		r.URL.RawPath = stripPathToEndOfV1(r.URL.RawPath)

		// Call the normal director for rewrites.
		nominalDirector(r)

		// Remove any forwarding headers added by the nominal director.
		r.Header.Del("X-Forwarded-For")
		r.Header.Del("X-Forwarded-Host")
		r.Header.Del("X-Forwarded-Proto")

		// All of OpenAI's models currently have a fixed context length, and there's
		// no notion of "runtime flags" that would be passed to their API (any
		// configuration would be done via the API), so we can skip processing the
		// configuration for now, but if we eventually expand the configuration to
		// include (say) default request parameters, then we could make that
		// adjustment to the request here.
	}

	// Set up proxy error logging.
	serverLogStream := o.serverLog.Writer()
	defer serverLogStream.Close()
	proxyAndServerLog := logpkg.New(serverLogStream, "", 0)
	proxy.ErrorLog = proxyAndServerLog

	// Listen for requests on the assigned Unix domain socket.
	os.Remove(socket)
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "unix", socket)
	if err != nil {
		return fmt.Errorf("unable to listen on backend socket: %w", err)
	}
	defer listener.Close()

	// Create an HTTP server to host the reverse proxy.
	server := &http.Server{
		Handler:  proxy,
		ErrorLog: proxyAndServerLog,
	}
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.Serve(listener)
	}()
	defer server.Close()

	// Poll for termination conditions.
	select {
	case <-ctx.Done():
		return nil
	case serverErr := <-serverErrors:
		return fmt.Errorf("OpenAI proxying terminated unexpectedly: %w", serverErr)
	}
}

// Status implements inference.Backend.Status.
func (*openai) Status() string {
	return "ready"
}

// GetDiskUsage implements inference.Backend.GetDiskUsage.
func (*openai) GetDiskUsage() (int64, error) {
	return 0, nil
}
