package metrics

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/docker/model-runner/pkg/distribution/oci/authn"
	"github.com/docker/model-runner/pkg/distribution/oci/reference"
	"github.com/docker/model-runner/pkg/distribution/registry"
	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/docker/model-runner/pkg/internal/utils"
	"github.com/docker/model-runner/pkg/logging"
	"github.com/sirupsen/logrus"
)

type Tracker struct {
	doNotTrack bool
	transport  http.RoundTripper
	log        logging.Logger
	userAgent  string
}

type TrackerRoundTripper struct {
	Transport http.RoundTripper
}

func (h *TrackerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
	defer cancel()
	clonedReq := req.Clone(ctx)
	clonedReq.Header.Set("x-docker-model-runner", "true")
	return h.Transport.RoundTrip(clonedReq)
}

func NewTracker(httpClient *http.Client, log logging.Logger, userAgent string, doNotTrack bool) *Tracker {
	client := *httpClient
	if client.Transport == nil {
		client.Transport = http.DefaultTransport
	}

	userAgent = strings.TrimSpace(userAgent)
	if userAgent == "" {
		userAgent = "docker-model-runner"
	} else {
		userAgent = userAgent + " docker-model-runner"
	}

	if os.Getenv("DEBUG") == "1" {
		if logger, ok := log.(*logrus.Logger); ok {
			logger.SetLevel(logrus.DebugLevel)
		} else if entry, ok := log.(*logrus.Entry); ok {
			entry.Logger.SetLevel(logrus.DebugLevel)
		}
	}

	return &Tracker{
		doNotTrack: os.Getenv("DO_NOT_TRACK") == "1" || doNotTrack,
		transport:  &TrackerRoundTripper{Transport: client.Transport},
		log:        log,
		userAgent:  userAgent,
	}
}

func (t *Tracker) TrackModel(model types.Model, userAgent, action string) {
	if t.doNotTrack {
		return
	}

	go t.trackModel(model, userAgent, action)
}

func (t *Tracker) trackModel(model types.Model, userAgent, action string) {
	tags := model.Tags()
	t.log.Debugln("Tracking model:", tags)
	if len(tags) == 0 {
		return
	}
	parts := []string{t.userAgent}
	if userAgent != "" {
		parts = append(parts, userAgent)
	}
	if action != "" {
		parts = append(parts, action)
	}
	ua := strings.Join(parts, " ")
	for _, tag := range tags {
		ref, err := reference.ParseReference(tag, registry.GetDefaultRegistryOptions()...)
		if err != nil {
			t.log.Errorf("Error parsing reference: %v\n", err)
			return
		}
		if err = t.headManifest(ref, ua); err != nil {
			t.log.Debugf("Manifest does not exist or error occurred: %v\n", err)
			continue
		}
		t.log.Debugln("Tracked", utils.SanitizeForLog(ref.Name(), -1), utils.SanitizeForLog(ref.Identifier(), -1), "with user agent:", utils.SanitizeForLog(ua, -1))
	}
}

// headManifest sends a HEAD request to check if the manifest exists
func (t *Tracker) headManifest(ref reference.Reference, ua string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Build the manifest URL
	registryHost := ref.Context().Registry.RegistryStr()
	if registryHost == "docker.io" || registryHost == "index.docker.io" {
		registryHost = "registry-1.docker.io"
	}
	repo := ref.Context().Repository
	identifier := ref.Identifier()

	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registryHost, repo, identifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, http.NoBody)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json")

	// Try to get credentials from keychain
	auth, err := authn.DefaultKeychain.Resolve(authn.NewResource(ref))
	if err == nil && auth != nil {
		if cfg, err := auth.Authorization(); err == nil {
			if cfg.Username != "" && cfg.Password != "" {
				req.SetBasicAuth(cfg.Username, cfg.Password)
			} else if cfg.RegistryToken != "" {
				req.Header.Set("Authorization", "Bearer "+cfg.RegistryToken)
			}
		}
	}

	resp, err := t.transport.RoundTrip(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("manifest not found: %d", resp.StatusCode)
	}

	return nil
}
