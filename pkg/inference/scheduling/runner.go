package scheduling

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	logpkg "log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"time"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/internal/utils"
	"github.com/docker/model-runner/pkg/logging"
	"github.com/docker/model-runner/pkg/metrics"
)

const (
	// maximumReadinessPings is the maximum number of retries that a runner will
	// perform when pinging a backend for readiness.
	maximumReadinessPings = 600
	// readinessRetryInterval is the interval at which a runner will retry
	// readiness checks for a backend.
	readinessRetryInterval = 500 * time.Millisecond
	// tcpBackendBasePort is the base port number for TCP-based backends.
	// Each slot gets a unique port: basePort + slot (e.g., 30000, 30001, 30002).
	// Port 30000+ is used to avoid conflicts with common services.
	tcpBackendBasePort = 30000
)

// errBackendNotReadyInTime indicates that an inference backend took too
// long to initialize and respond to a readiness request.
var errBackendNotReadyInTime = errors.New("inference backend took too long to initialize")

// errBackendQuitUnexpectedly indicates that an inference backend terminated
// unexpectedly
var errBackendQuitUnexpectedly = errors.New("inference backend quit unexpectedly")

// RunnerSocketPath determines the Unix domain socket path used to communicate
// with runners at the specified slot. It can be overridden during init().
var RunnerSocketPath = func(slot int) (string, error) {
	return fmt.Sprintf("inference-runner-%d.sock", slot), nil
}

// runner executes a given backend with a given model and provides reverse
// proxying to that backend.
type runner struct {
	// log is the component logger.
	log logging.Logger
	// backend is the associated backend.
	backend inference.Backend
	// model is the associated model.
	model string
	// mode is the backend operation mode.
	mode inference.BackendMode
	// cancel terminates the runner's backend run loop.
	cancel context.CancelFunc
	// done is closed when the runner's backend run loop exits.
	done <-chan struct{}
	// transport is a transport targeting the runner's socket.
	transport *http.Transport
	// client is a client targeting the runner's HTTP server.
	client *http.Client
	// proxy is a reverse proxy targeting the runner's HTTP server.
	proxy *httputil.ReverseProxy
	// proxyLog is the stream used for logging by proxy.
	proxyLog io.Closer
	// openAIRecorder is used to record OpenAI API inference requests and responses.
	openAIRecorder *metrics.OpenAIRecorder
	// err is the error returned by the runner's backend, only valid after done is closed.
	err error
}

// run creates a new runner instance.
func run(
	log logging.Logger,
	backend inference.Backend,
	modelID string,
	modelRef string,
	mode inference.BackendMode,
	slot int,
	runnerConfig *inference.BackendConfiguration,
	openAIRecorder *metrics.OpenAIRecorder,
) (*runner, error) {
	// Create a dialer / transport that target backend on the specified slot.
	network := "tcp"
	socket := net.JoinHostPort("127.0.0.1", strconv.Itoa(tcpBackendBasePort+slot))

	if !backend.UsesTCP() {
		var err error
		socket, err = RunnerSocketPath(slot)
		if err != nil {
			return nil, fmt.Errorf("unable to determine runner socket path: %w", err)
		}
		network = "unix"
	}

	dialer := &net.Dialer{}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, socket)
		},
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// Create a client that we can use internally to ping the backend.
	client := &http.Client{Transport: transport}

	// Create a reverse proxy targeting the backend. The virtual URL that we use
	// here is merely a placeholder; the transport always dials the backend HTTP
	// endpoint and the hostname is always overwritten in the proxy. This URL is
	// not accessible from anywhere.
	upstream, err := url.Parse("http://inference.docker.internal")
	if err != nil {
		return nil, fmt.Errorf("unable to parse virtual backend URL: %w", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	standardDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		standardDirector(r)
		// HACK: Most backends will be happier with a "localhost" hostname than
		// an "inference.docker.internal" hostname (which they may reject).
		r.Host = "localhost"
		// Remove the prefix up to the OpenAI API root.
		r.URL.Path = trimRequestPathToOpenAIRoot(r.URL.Path)
		r.URL.RawPath = trimRequestPathToOpenAIRoot(r.URL.RawPath)
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		// CORS headers are set by the CorsMiddleware from pkg/inference/cors.go,
		// so we remove them here to avoid duplication and potential misconfiguration.
		resp.Header.Del("Access-Control-Allow-Origin")
		return nil
	}
	proxy.Transport = transport
	proxyLog := log.Writer()
	proxy.ErrorLog = logpkg.New(proxyLog, "", 0)

	// Create a cancellable context to regulate the runner's backend run loop
	// and a channel to track its termination.
	runCtx, runCancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})

	r := &runner{
		log:            log,
		backend:        backend,
		model:          modelID,
		mode:           mode,
		cancel:         runCancel,
		done:           runDone,
		transport:      transport,
		client:         client,
		proxy:          proxy,
		proxyLog:       proxyLog,
		openAIRecorder: openAIRecorder,
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
		// If the error is EOF, the underlying runner likely bailed, and closed its socket
		// unexpectedly. Wait for the runner process to complete, but time out in case
		// the runner process only killed its comms and is stuck.
		if errors.Is(err, io.EOF) {
			w.WriteHeader(http.StatusInternalServerError)
			select {
			case <-r.done:
				res := OpenAIErrorResponse{
					Type:           "error",
					Code:           nil,
					Message:        r.err.Error(),
					Param:          nil,
					SequenceNumber: 1,
				}
				errJson, err := json.Marshal(&res)
				if err == nil {
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.Write(errJson)
				}
				return
			case <-time.After(30 * time.Second):
			}
		} else {
			w.WriteHeader(http.StatusBadGateway)
		}
	}
	if r.openAIRecorder != nil {
		r.openAIRecorder.SetConfigForModel(modelID, runnerConfig)
	} else {
		r.log.Warnf("OpenAI recorder is nil for model %s", modelID)
	}

	// Start the backend run loop.
	go func() {
		if err := backend.Run(runCtx, socket, modelID, modelRef, mode, runnerConfig); err != nil {
			log.Warnf("Backend %s running model %s exited with error: %v",
				backend.Name(), utils.SanitizeForLog(modelRef), err,
			)
			r.err = err
		}
		close(runDone)
	}()

	// Create the runner.
	return r, nil
}

// wait waits for the runner to be ready.
func (r *runner) wait(ctx context.Context) error {
	// Loop and poll for readiness.
	for p := 0; p < maximumReadinessPings; p++ {
		select {
		case <-r.done:
			if r.err == nil {
				return errBackendQuitUnexpectedly
			}
			return r.err
		default:
		}
		// Create and execute a request targeting a known-valid endpoint.
		readyRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/v1/models", http.NoBody)
		if err != nil {
			return fmt.Errorf("readiness request creation failed: %w", err)
		}
		response, err := r.client.Do(readyRequest)
		if err == nil {
			response.Body.Close()
		}

		// If the request failed, then wait (if appropriate) and try again.
		if err != nil || response.StatusCode != http.StatusOK {
			if p < (maximumReadinessPings - 1) {
				select {
				case <-time.After(readinessRetryInterval):
					continue
				case <-ctx.Done():
					return context.Canceled
				}
			}
			break
		}

		// The backend responded successfully.
		return nil
	}

	// The backend did not initialize and respond in time.
	return errBackendNotReadyInTime
}

// terminate stops the runner instance and waits for it to unload from memory.
func (r *runner) terminate() {
	// Signal termination and wait for the run loop to exit.
	r.cancel()
	<-r.done

	// Close any idle connections.
	r.client.CloseIdleConnections()
	r.transport.CloseIdleConnections()

	// Close the proxy's log.
	if err := r.proxyLog.Close(); err != nil {
		r.log.Warnf("Unable to close reverse proxy log writer: %v", err)
	}

	if r.openAIRecorder != nil {
		r.openAIRecorder.RemoveModel(r.model)
	} else {
		r.log.Warnf("OpenAI recorder is nil for model %s", r.model)
	}
}

// ServeHTTP implements net/http.Handler.ServeHTTP. It forwards requests to the
// backend's HTTP server.
func (r *runner) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.proxy.ServeHTTP(w, req)
}
