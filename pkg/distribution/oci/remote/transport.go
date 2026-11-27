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

// cloudMetadataIPs are the well-known instance-metadata service addresses.
// They must never be contacted as a token endpoint, even when the realm is on
// the registry's own authority: a registry able to rebind its hostname (or a
// proxy resolving it independently) could otherwise use the trust-domain
// exception to reach them. These are deliberately a narrow set of addresses,
// not the whole link-local/private range, so internal registries on RFC1918
// addresses keep working (see issue #1050).
var cloudMetadataIPs = func() []net.IP {
	addresses := []string{
		"169.254.169.254", // AWS/GCP/Azure/OpenStack IMDSv1 & v2
		"169.254.170.2",   // AWS ECS task metadata
		"fd00:ec2::254",   // AWS IMDS over IPv6
		"100.100.100.200", // Alibaba Cloud metadata
	}
	ips := make([]net.IP, 0, len(addresses))
	for _, address := range addresses {
		ip := net.ParseIP(address)
		if ip == nil {
			panic(fmt.Sprintf("internal error: failed to parse hardcoded metadata IP %q", address))
		}
		ips = append(ips, ip)
	}
	return ips
}()

// metadataHostnames are metadata service names that must never be used as a
// realm, regardless of what address they resolve to.
var metadataHostnames = []string{
	"metadata.google.internal",
	"metadata.goog",
}

// isMetadataIP reports whether ip is a known cloud instance-metadata address.
func isMetadataIP(ip net.IP) bool {
	for _, metadata := range cloudMetadataIPs {
		if metadata.Equal(ip) {
			return true
		}
	}
	return false
}

// isMetadataHostname reports whether hostname names a metadata service. A
// trailing dot is the canonical absolute-DNS spelling of the same name, so it
// is stripped before comparing.
func isMetadataHostname(hostname string) bool {
	hostname = strings.TrimRight(hostname, ".")
	for _, name := range metadataHostnames {
		if strings.EqualFold(hostname, name) {
			return true
		}
	}
	return false
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

// isTrustedDockerTokenEndpoint recognizes Docker Hub's token endpoint only when
// authenticating to Docker Hub. Exact HTTPS authority and path matching avoids
// extending this exception to arbitrary hosts, ports, or authentication services.
func isTrustedDockerTokenEndpoint(u *url.URL, registryHost string) bool {
	switch strings.ToLower(registryHost) {
	case "docker.io", "index.docker.io", "registry-1.docker.io":
	default:
		return false
	}
	return u.Scheme == "https" && u.User == nil && u.Fragment == "" &&
		(strings.EqualFold(u.Host, "auth.docker.io") || strings.EqualFold(u.Host, "auth.docker.io:443")) &&
		u.EscapedPath() == "/token"
}

// normalizeAuthority returns host as a canonical "host:port" authority, filling
// in the scheme's default port when host carries none. The trust domain is the
// full authority, not the bare hostname: the same IP on a different port is a
// different service, so comparing hostnames alone would reopen the SSRF.
func normalizeAuthority(host, scheme string) string {
	if host == "" {
		return ""
	}
	h, port, err := net.SplitHostPort(host)
	if err != nil {
		// No port at all. A bracketed IPv6 literal ("[::1]") must lose its
		// brackets before net.JoinHostPort re-adds them, otherwise the result
		// is double-bracketed ("[[::1]]:80") and never matches the dialer's
		// "[::1]:80".
		h = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
		port = ""
	}
	if port == "" {
		switch scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}
	if port == "" {
		return h
	}
	return net.JoinHostPort(h, port)
}

// isTrustedAuthority reports whether a normalized realm authority equals the
// single trusted authority of the registry being contacted. Only that one
// authority is compared: probing both http and https here would let an HTTPS
// registry implicitly trust an unrelated HTTP (port 80) service on the same
// hostname, which is a different service and must stay blocked.
func isTrustedAuthority(realmAuthority, trustedAuthority string) bool {
	return trustedAuthority != "" && strings.EqualFold(realmAuthority, trustedAuthority)
}

// isSameAuthorityRealm reports whether the realm URL points at the exact same
// authority (host and port) as the registry being contacted. Such a token
// endpoint is inside the registry's own trust domain, so its private/loopback/
// link-local address is legitimate (internal and self-hosted registries).
// registryScheme is the scheme actually used to contact the registry; only its
// effective port is trusted, never the other scheme's default port.
func isSameAuthorityRealm(u *url.URL, registryHost, registryScheme string) bool {
	trusted := normalizeAuthority(registryHost, registryScheme)
	return isTrustedAuthority(normalizeAuthority(u.Host, u.Scheme), trusted)
}

// lookupHostAddrs returns the addresses hostname resolves to, or hostname itself
// when it is already a literal IP.
func lookupHostAddrs(hostname string) ([]string, error) {
	if ip := net.ParseIP(hostname); ip != nil {
		return []string{hostname}, nil
	}
	ips, err := net.LookupHost(hostname)
	if err != nil {
		return nil, fmt.Errorf("resolving realm hostname %q: %w", hostname, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("realm hostname %q resolved to no addresses", hostname)
	}
	return ips, nil
}

// resolveHost resolves a realm on the registry's own authority to a pinned dial
// address. Private/loopback/link-local addresses are permitted there because
// internal registries legitimately use them, but cloud metadata addresses are
// never allowed. The resolved address is pinned so the connection goes exactly
// where validation looked.
func resolveHost(hostname, port string) (string, error) {
	return resolveCheckedHost(hostname, port, true)
}

// effectivePort returns u's explicit port, or the default for its scheme.
func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

// validateTokenEndpointURL validates a token endpoint before any request is
// made. Cloud metadata addresses are always rejected, including for realms on
// the registry's own authority. Same-authority realms still resolve and check
// their destination, because the dialer's pinning does not run for proxied
// requests; only Docker Hub's trusted HTTPS endpoint skips local DNS entirely
// (a proxy may resolve it where public DNS is unavailable).
//
// Known limitation: the same-authority exception is keyed to the realm's
// hostname, not to the address the registry's data-plane connection was
// validated against. A hostname rebound between the registry request and the
// token request to a private address other than a known metadata address is
// still followed, and for proxied requests the proxy resolves the destination
// independently of the local validation performed here, so client-side pinning
// cannot cover that path. Closing this fully requires carrying the validated
// registry connection address into this client; the guard deliberately stops
// at the narrower contract above (metadata always blocked, other private
// addresses allowed only for the contacted host's own authority).
func validateTokenEndpointURL(u *url.URL, registryHost, registryScheme string) error {
	if isTrustedDockerTokenEndpoint(u, registryHost) {
		return nil
	}
	port := effectivePort(u)
	allowPrivate := isSameAuthorityRealm(u, registryHost, registryScheme)
	_, err := resolveCheckedHost(u.Hostname(), port, allowPrivate)
	return err
}

// resolveAndValidateHost validates hostname against the internal-hostname
// blocklist and the private/loopback/link-local IP ranges, returning a pinned
// dial address (ip:port) that is safe to connect to.
func resolveAndValidateHost(hostname, port string) (dialAddr string, err error) {
	return resolveCheckedHost(hostname, port, false)
}

// resolveCheckedHost is the single DNS-resolution and destination-validation
// path for token endpoints. It resolves once, validates every returned address,
// and returns a pinned dial address. When allowPrivate is true (only for realms
// on the registry's own authority) private/loopback/link-local addresses are
// permitted; cloud metadata addresses and metadata hostnames are rejected in
// both modes.
func resolveCheckedHost(hostname, port string, allowPrivate bool) (string, error) {
	if isMetadataHostname(hostname) {
		return "", fmt.Errorf("realm URL hostname %q is not allowed", hostname)
	}
	if !allowPrivate {
		for _, internal := range internalHostnames {
			if strings.EqualFold(hostname, internal) {
				return "", fmt.Errorf("realm URL hostname %q is not allowed", hostname)
			}
		}
	}

	addrs, err := lookupHostAddrs(hostname)
	if err != nil {
		return "", err
	}
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			continue
		}
		if isMetadataIP(ip) {
			return "", fmt.Errorf("realm URL contains a disallowed IP address %s (cloud metadata endpoint)", addr)
		}
		if !allowPrivate && isDisallowedIP(ip) {
			return "", fmt.Errorf("realm URL resolves to a disallowed address %s", addr)
		}
	}

	return net.JoinHostPort(addrs[0], port), nil
}

// newGuardedAuthClient returns the HTTP client used to fetch bearer tokens,
// both by containerd's authorizer (via docker.WithAuthClient) and by the
// hand-rolled Exchange(). The realm URL in a registry's WWW-Authenticate
// challenge is attacker-controlled, so every request this client resolves and
// validates its destination before connecting. Realms on the registry's own
// authority may use private addresses (internal registries), but cloud metadata
// addresses are always blocked. Docker Hub's trusted token endpoint is the only
// path that skips local DNS, for proxied setups without public DNS.
//
// How the connection is guarded depends on whether a proxy applies to the
// request (see guardedAuthTransport). A dial-time-only guard would break every
// proxied deployment: with a proxy configured, the dialer sees the proxy's
// address — commonly a private or loopback IP — rather than the realm's, and
// would reject the proxy itself.
func newGuardedAuthClient(base http.RoundTripper, registryHost, registryScheme string) *http.Client {
	// Only the authority actually contacted (host + effective port for the
	// registry's scheme) is trusted. Computing it once here keeps the dialer
	// and the proxied request path on the same, single trust domain.
	trustedAuthority := normalizeAuthority(registryHost, registryScheme)
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
		if isTrustedAuthority(normalizeAuthority(addr, ""), trustedAuthority) {
			// Realm is the registry's own authority: private addresses are
			// allowed, but the destination is still resolved, validated (cloud
			// metadata is always rejected) and pinned (see validateTokenEndpointURL).
			dialAddr, err := resolveHost(host, port)
			if err != nil {
				return nil, err
			}
			return (&net.Dialer{}).DialContext(ctx, network, dialAddr)
		}
		dialAddr, err := resolveAndValidateHost(host, port)
		if err != nil {
			return nil, err
		}
		return (&net.Dialer{}).DialContext(ctx, network, dialAddr)
	}

	return &http.Client{Transport: &guardedAuthTransport{proxied: proxied, direct: direct, registryHost: registryHost, registryScheme: registryScheme}}
}

// guardedAuthTransport validates every token-endpoint request against the SSRF
// blocklist, choosing the enforcement point per request:
//
//   - Direct connections use a transport whose dialer validates the resolved
//     IP just before connecting and dials that exact address, so DNS rebinding
//     cannot slip an internal address past the check.
//   - Proxied connections go through a transport with the proxy configuration
//     intact and the supplied dialer: the proxy is the one connecting to the
//     realm, so pinning the dial address is neither possible nor meaningful.
//     The realm destination is validated here at the request level instead,
//     including metadata blocking for same-authority realms. Redirects are
//     checked as well.
type guardedAuthTransport struct {
	proxied        *http.Transport // proxy settings and supplied dialer intact
	direct         *http.Transport // no proxy, validating dialer pinned to the resolved IP
	registryHost   string          // host of the registry being contacted (Docker Hub exception)
	registryScheme string          // scheme actually used to contact the registry
}

func (g *guardedAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if g.proxied.Proxy != nil {
		proxyURL, err := g.proxied.Proxy(req)
		if err != nil {
			return nil, fmt.Errorf("determining proxy for token endpoint: %w", err)
		}
		if proxyURL != nil {
			if err := validateTokenEndpointURL(req.URL, g.registryHost, g.registryScheme); err != nil {
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
func Exchange(ctx context.Context, reg reference.Registry, auth authn.Authenticator, transport http.RoundTripper, scopes []string, pr *PingResponse) (*Token, error) {
	client := newGuardedAuthClient(transport, reg.RegistryStr(), reg.Scheme())

	// Build token request URL
	tokenURL, err := url.Parse(pr.WWWAuthenticate.Realm)
	if err != nil {
		return nil, fmt.Errorf("parsing realm URL: %w", err)
	}

	// Validate the realm before any request is made so a blocked realm fails
	// fast with a clear error. The guarded client re-validates at connection
	// time (or per request when proxied), closing the TOCTOU window.
	if err := validateTokenEndpointURL(tokenURL, reg.RegistryStr(), reg.Scheme()); err != nil {
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
