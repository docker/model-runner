package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
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

// privateOrLoopbackCIDRs lists IP ranges that must never be contacted as a
// token-exchange realm. Allowing requests to these addresses would let a
// malicious registry pivot Model Runner into an internal-service proxy
// (SSRF), reaching endpoints that are not accessible from the public internet.
var privateOrLoopbackCIDRs = func() []*net.IPNet {
	cidrs := []string{
		"127.0.0.0/8",    // loopback IPv4
		"::1/128",        // loopback IPv6
		"169.254.0.0/16", // link-local IPv4 / AWS EC2 instance-metadata
		"fe80::/10",      // link-local IPv6
		"10.0.0.0/8",     // RFC-1918 private
		"172.16.0.0/12",  // RFC-1918 private
		"192.168.0.0/16", // RFC-1918 private
		"fc00::/7",       // IPv6 ULA
	}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			// These are hardcoded compile-time constants; a parse failure
			// indicates a programmer error (e.g. a typo). Panic immediately
			// so the mistake is caught at startup rather than silently
			// weakening the SSRF blocklist.
			panic(fmt.Sprintf("internal error: failed to parse hardcoded CIDR %q: %v", cidr, err))
		}
		nets = append(nets, n)
	}
	return nets
}()

// internalHostnames lists hostnames that must never be used as a realm,
// regardless of what IP address they resolve to.
var internalHostnames = []string{
	"localhost",
	"host.docker.internal",
	"model-runner.docker.internal",
	"gateway.docker.internal",
}

// isDisallowedIP reports whether ip falls in any of the private/loopback/
// link-local ranges that must not be contacted as a token-exchange realm.
func isDisallowedIP(ip net.IP) bool {
	for _, cidr := range privateOrLoopbackCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// validateTokenEndpointURL validates the host of a token-endpoint URL against
// the internal-hostname blocklist and the private/loopback/link-local ranges.
// The local DNS resolution this performs is deliberate even when a proxy will
// resolve the name itself: checking the resolved IPs is the validation, and a
// name that cannot be resolved locally is rejected (fail closed) rather than
// forwarded unchecked.
func validateTokenEndpointURL(u *url.URL) error {
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	_, err := resolveAndValidateHost(u.Hostname(), port)
	return err
}

// resolveAndValidateHost validates hostname against the internal-hostname
// blocklist and the private/loopback/link-local IP ranges, returning a dial
// address (ip:port) that is safe to connect to. Returning the resolved IP lets
// callers dial that exact address, closing the DNS-rebinding (TOCTOU) window
// between validation and connection. A literal IP hostname is validated
// directly without a DNS lookup.
func resolveAndValidateHost(hostname, port string) (dialAddr string, err error) {
	for _, internal := range internalHostnames {
		if strings.EqualFold(hostname, internal) {
			return "", fmt.Errorf("realm URL hostname %q is not allowed", hostname)
		}
	}

	if ip := net.ParseIP(hostname); ip != nil {
		if isDisallowedIP(ip) {
			return "", fmt.Errorf("realm URL contains a disallowed IP address %s", hostname)
		}
		return net.JoinHostPort(hostname, port), nil
	}

	ips, err := net.LookupHost(hostname)
	if err != nil {
		return "", fmt.Errorf("resolving realm hostname %q: %w", hostname, err)
	}
	if len(ips) == 0 {
		return "", fmt.Errorf("realm hostname %q resolved to no addresses", hostname)
	}
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if isDisallowedIP(ip) {
			return "", fmt.Errorf("realm URL resolves to a disallowed address %s", ipStr)
		}
	}

	return net.JoinHostPort(ips[0], port), nil
}

// newGuardedAuthClient returns the HTTP client used to fetch bearer tokens,
// both by containerd's authorizer (via docker.WithAuthClient) and by the
// hand-rolled Exchange(). The realm URL in a registry's WWW-Authenticate
// challenge is attacker-controlled, so every request this client makes is
// validated against the internal-hostname blocklist and the private/loopback/
// link-local IP ranges before a connection is established.
//
// How the connection is guarded depends on whether a proxy applies to the
// request (see guardedAuthTransport). A dial-time-only guard would break every
// proxied deployment: with a proxy configured, the dialer sees the proxy's
// address — commonly a private or loopback IP — rather than the realm's, and
// would reject the proxy itself.
func newGuardedAuthClient(base http.RoundTripper) *http.Client {
	var proxied *http.Transport
	if t, ok := base.(*http.Transport); ok {
		proxied = t.Clone()
	} else if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		proxied = dt.Clone()
	} else {
		proxied = &http.Transport{}
	}

	direct := proxied.Clone()
	direct.Proxy = nil
	direct.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid token endpoint address %q: %w", addr, err)
		}
		dialAddr, err := resolveAndValidateHost(host, port)
		if err != nil {
			return nil, err
		}
		return (&net.Dialer{}).DialContext(ctx, network, dialAddr)
	}

	return &http.Client{Transport: &guardedAuthTransport{proxied: proxied, direct: direct}}
}

// guardedAuthTransport validates every token-endpoint request against the SSRF
// blocklist, choosing the enforcement point per request:
//
//   - Direct connections use a transport whose dialer validates the resolved
//     IP just before connecting and dials that exact address, so DNS rebinding
//     cannot slip an internal address past the check.
//   - Proxied connections go through a transport with the proxy configuration
//     intact and a stock dialer: the proxy is the one connecting to the realm,
//     so pinning the dial address is neither possible nor meaningful. The
//     realm host is validated here at the request level instead.
type guardedAuthTransport struct {
	proxied *http.Transport // proxy settings intact, stock dialer
	direct  *http.Transport // no proxy, validating dialer pinned to the resolved IP
}

func (g *guardedAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if g.proxied.Proxy != nil {
		proxyURL, err := g.proxied.Proxy(req)
		if err != nil {
			return nil, fmt.Errorf("determining proxy for token endpoint: %w", err)
		}
		if proxyURL != nil {
			if err := validateTokenEndpointURL(req.URL); err != nil {
				return nil, fmt.Errorf("realm URL rejected: %w", err)
			}
			return g.proxied.RoundTrip(req)
		}
	}
	return g.direct.RoundTrip(req)
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

// Exchange exchanges credentials for a bearer token. The realm URL comes from
// the registry's WWW-Authenticate challenge and is therefore untrusted; the
// guarded client rejects realms on internal hostnames or private/loopback
// addresses and honors any configured proxy.
func Exchange(ctx context.Context, _ reference.Registry, auth authn.Authenticator, transport http.RoundTripper, scopes []string, pr *PingResponse) (*Token, error) {
	client := newGuardedAuthClient(transport)

	// Build token request URL
	tokenURL, err := url.Parse(pr.WWWAuthenticate.Realm)
	if err != nil {
		return nil, fmt.Errorf("parsing realm URL: %w", err)
	}

	// Validate the realm before any request is made so a blocked realm fails
	// fast with a clear error. The guarded client re-validates at connection
	// time (or per request when proxied), closing the TOCTOU window.
	if err := validateTokenEndpointURL(tokenURL); err != nil {
		return nil, fmt.Errorf("realm URL rejected: %w", err)
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
		slog.DebugContext(ctx, "token request failed",
			"status", resp.StatusCode,
			"body", string(body),
		)
		return nil, fmt.Errorf("token request failed: unexpected status %d from token endpoint", resp.StatusCode)
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
