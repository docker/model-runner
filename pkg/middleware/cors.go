package middleware

import (
	"net/http"

	"github.com/docker/model-runner/pkg/envconfig"
)

// CorsMiddleware handles CORS and OPTIONS preflight requests with optional allowedOrigins.
// If allowedOrigins is nil or empty, it falls back to envconfig.AllowedOrigins().
// This middleware intercepts OPTIONS requests only if the Origin header is present and valid,
// otherwise passing the request to the router (allowing 405/404 responses as appropriate).
func CorsMiddleware(allowedOrigins []string, next http.Handler) http.Handler {
	if len(allowedOrigins) == 0 {
		allowedOrigins = envconfig.AllowedOrigins()
	}

	allowAll := len(allowedOrigins) == 1 && allowedOrigins[0] == "*"
	allowedSet := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowedSet[o] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		allowed := allowAll || originAllowed(origin, allowedSet)

		if origin != "" && !allowed {
			http.Error(w, "Origin not allowed", http.StatusForbidden)
			return
		}

		// Set CORS headers if origin is allowed
		if origin != "" && allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}

		// Handle OPTIONS requests with origin validation.
		// Only intercept OPTIONS if the origin is valid to prevent unauthorized preflight requests.
		if r.Method == http.MethodOptions {
			// Require valid Origin header for OPTIONS requests
			if origin == "" || !allowed {
				// No origin or invalid origin - pass to router for proper 405/404 response
				next.ServeHTTP(w, r)
				return
			}

			// Valid origin - handle OPTIONS with CORS headers
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "*")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func originAllowed(origin string, allowedSet map[string]struct{}) bool {
	_, ok := allowedSet[origin]
	return ok
}
