package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/docker/model-runner/pkg/distribution/oci/authn"
	"github.com/docker/model-runner/pkg/distribution/oci/reference"
)

// PullScope is the scope for pulling from a registry.
const PullScope = "pull"

// PushScope is the scope for pushing to a registry.
const PushScope = "push,pull"

// PingResponse contains information from a registry ping.
type PingResponse struct {
	WWWAuthenticate WWWAuthenticate
}

// WWWAuthenticate contains parsed WWW-Authenticate header information.
type WWWAuthenticate struct {
	Realm   string
	Service string
	Scope   string
}

// Token represents an authentication token.
type Token struct {
	Token       string `json:"token"`
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// Ping pings a registry and returns authentication information.
func Ping(ctx context.Context, reg reference.Registry, transport http.RoundTripper) (*PingResponse, error) {
	if transport == nil {
		transport = http.DefaultTransport
	}

	client := &http.Client{Transport: transport}
	scheme := reg.Scheme()

	pingURL := fmt.Sprintf("%s://%s/v2/", scheme, reg.RegistryStr())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pingURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating ping request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pinging registry: %w", err)
	}
	defer resp.Body.Close()

	// Parse WWW-Authenticate header
	wwwAuth := resp.Header.Get("WWW-Authenticate")
	if wwwAuth == "" {
		// No auth required or already authenticated
		return &PingResponse{}, nil
	}

	pr := &PingResponse{
		WWWAuthenticate: parseWWWAuthenticate(wwwAuth),
	}

	return pr, nil
}

// parseWWWAuthenticate parses a WWW-Authenticate header.
func parseWWWAuthenticate(header string) WWWAuthenticate {
	result := WWWAuthenticate{}

	// Remove "Bearer " prefix
	header = strings.TrimPrefix(header, "Bearer ")

	// Parse key=value pairs
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		value := strings.Trim(strings.TrimSpace(kv[1]), "\"")

		switch key {
		case "realm":
			result.Realm = value
		case "service":
			result.Service = value
		case "scope":
			result.Scope = value
		}
	}

	return result
}

// Exchange exchanges credentials for a bearer token.
func Exchange(ctx context.Context, reg reference.Registry, auth authn.Authenticator, transport http.RoundTripper, scopes []string, pr *PingResponse) (*Token, error) {
	if transport == nil {
		transport = http.DefaultTransport
	}

	client := &http.Client{Transport: transport}

	// Build token request URL
	tokenURL, err := url.Parse(pr.WWWAuthenticate.Realm)
	if err != nil {
		return nil, fmt.Errorf("parsing realm URL: %w", err)
	}

	q := tokenURL.Query()
	if pr.WWWAuthenticate.Service != "" {
		q.Set("service", pr.WWWAuthenticate.Service)
	}
	for _, scope := range scopes {
		q.Add("scope", scope)
	}
	tokenURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL.String(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating token request: %w", err)
	}

	// Add authentication if provided
	if auth != nil {
		cfg, err := auth.Authorization()
		if err != nil {
			return nil, fmt.Errorf("getting auth config: %w", err)
		}
		if cfg.Username != "" && cfg.Password != "" {
			req.SetBasicAuth(cfg.Username, cfg.Password)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var token Token
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, fmt.Errorf("decoding token response: %w", err)
	}

	// Some registries return access_token instead of token
	if token.Token == "" && token.AccessToken != "" {
		token.Token = token.AccessToken
	}

	return &token, nil
}

// BearerTransport wraps an http.RoundTripper with bearer token authentication.
type BearerTransport struct {
	Transport http.RoundTripper
	Token     string
}

// RoundTrip implements http.RoundTripper.
func (t *BearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	if t.Token != "" {
		req2.Header.Set("Authorization", "Bearer "+t.Token)
	}
	if t.Transport == nil {
		return http.DefaultTransport.RoundTrip(req2)
	}
	return t.Transport.RoundTrip(req2)
}
