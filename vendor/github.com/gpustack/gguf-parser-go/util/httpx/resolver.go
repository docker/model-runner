package httpx

import (
	"context"
	"net"
	"time"

	"github.com/rs/dnscache"
)

// DefaultResolver is the default DNS resolver used by the package,
// which caches DNS lookups in memory.
var DefaultResolver = &dnscache.Resolver{
	// NB(thxCode): usually, a high latency DNS is about 3s,
	// so we set the timeout to 5s here.
	Timeout:  5 * time.Second,
	Resolver: net.DefaultResolver,
}

func init() {
	go func() {
		t := time.NewTimer(5 * time.Minute)
		defer t.Stop()
		for range t.C {
			DefaultResolver.RefreshWithOptions(dnscache.ResolverRefreshOptions{
				ClearUnused:      true,
				PersistOnFailure: false,
			})
		}
	}()
}

func DNSCacheDialContext(dialer *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, nw, addr string) (conn net.Conn, err error) {
		h, p, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := DefaultResolver.LookupHost(ctx, h)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			conn, err = dialer.DialContext(ctx, nw, net.JoinHostPort(ip, p))
			if err == nil {
				break
			}
		}
		return conn, err
	}
}
