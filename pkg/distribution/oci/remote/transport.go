package remote

import (
	"context"
	"crypto/tls"
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

// resolveAndValidateRealm parses the realm URL, validates the hostname against
// a static blocklist, and resolves it to a dial address (ip:port) that is safe
// to connect to. By returning the resolved IP, callers can use a custom
// DialContext to connect to that exact address — preventing DNS-rebinding
// attacks where a malicious DNS server could return different IPs for
// successive lookups (TOCTOU).
//
// If hostname is a literal IP address it is validated directly without
// triggering a DNS lookup.
func resolveAndValidateRealm(rawURL string) (dialAddr, hostname string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid realm URL: %w", err)
	}

	hostname = u.Hostname()
	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "https":
			port = "443"
		default:
			port = "80"
		}
	}

	// Block well-known internal hostnames regardless of DNS resolution.
	for _, internal := range internalHostnames {
		if strings.EqualFold(hostname, internal) {
			return "", "", fmt.Errorf("realm URL hostname %q is not allowed", hostname)
		}
	}

	// If the hostname is a literal IP address, validate it directly without a
	// DNS lookup — there is no DNS to rebind.
	if ip := net.ParseIP(hostname); ip != nil {
		if isDisallowedIP(ip) {
			return "", "", fmt.Errorf("realm URL contains a disallowed IP address %s", hostname)
		}
		return net.JoinHostPort(hostname, port), hostname, nil
	}

	// Resolve the hostname and validate every returned address. Using the
	// resolved IP as the dial address prevents DNS rebinding: the same IP that
	// passed validation is the one that will be used for the connection.
	ips, err := net.LookupHost(hostname)
	if err != nil {
		return "", "", fmt.Errorf("resolving realm hostname %q: %w", hostname, err)
	}
	if len(ips) == 0 {
		return "", "", fmt.Errorf("realm hostname %q resolved to no addresses", hostname)
	}
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if isDisallowedIP(ip) {
			return "", "", fmt.Errorf("realm URL resolves to a disallowed address %s", ipStr)
		}
	}

	// All resolved IPs passed validation. Use the first one as the dial
	// address so the HTTP client never performs a second DNS lookup.
	return net.JoinHostPort(ips[0], port), hostname, nil
}

// buildSafeTransport wraps base with a custom DialContext that connects
// directly to dialAddr (a pre-validated "ip:port") instead of relying on DNS
// resolution. This closes the TOCTOU window between realm URL validation and
// the actual connection. For TLS connections, serverName is set as the SNI
// value so that certificate validation still uses the original hostname.
func buildSafeTransport(base http.RoundTripper, dialAddr, serverName string) (http.RoundTripper, error) {
	if base == nil {
		base = http.DefaultTransport
	}

	t, ok := base.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("cannot build safe transport from base of type %T; only *http.Transport is supported", base)
	}

	dial := func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, dialAddr)
	}

	cloned := t.Clone()
	cloned.DialContext = dial
	if cloned.TLSClientConfig == nil {
		cloned.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12} //nolint:gosec
	} else {
		cloned.TLSClientConfig = cloned.TLSClientConfig.Clone()
	}
	cloned.TLSClientConfig.ServerName = serverName
	return cloned, nil
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
func Exchange(ctx context.Context, _ reference.Registry, auth authn.Authenticator, transport http.RoundTripper, scopes []string, pr *PingResponse) (*Token, error) {
	if transport == nil {
		transport = http.DefaultTransport
	}

	dialAddr, hostname, err := resolveAndValidateRealm(pr.WWWAuthenticate.Realm)
	if err != nil {
		return nil, fmt.Errorf("realm URL rejected: %w", err)
	}

	safeTransport, err := buildSafeTransport(transport, dialAddr, hostname)
	if err != nil {
		return nil, fmt.Errorf("failed to build safe transport: %w", err)
	}
	client := &http.Client{Transport: safeTransport}

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
