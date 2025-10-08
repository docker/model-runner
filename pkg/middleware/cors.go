package middleware

import (
	"net/http"
	"os"
	"strings"
)

// CorsMiddleware handles CORS and OPTIONS preflight requests with optional allowedOrigins.
// If allowedOrigins is nil or empty, it falls back to getAllowedOrigins().
// This middleware intercepts OPTIONS requests only if the Origin header is present and valid,
// otherwise passing the request to the router (allowing 405/404 responses as appropriate).
func CorsMiddleware(allowedOrigins []string, next http.Handler) http.Handler {
	if len(allowedOrigins) == 0 {
		allowedOrigins = getAllowedOrigins()
	}

	// Explicitly disable all origins.
	if allowedOrigins == nil {
		return next
	}

	allowAll := len(allowedOrigins) == 1 && allowedOrigins[0] == "*"
	allowedSet := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowedSet[o] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Set CORS headers if origin is allowed
		if origin != "" && (allowAll || originAllowed(origin, allowedSet)) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}

		// Handle OPTIONS requests with origin validation.
		// Only intercept OPTIONS if the origin is valid to prevent unauthorized preflight requests.
		if r.Method == http.MethodOptions {
			// Require valid Origin header for OPTIONS requests
			if origin == "" || !(allowAll || originAllowed(origin, allowedSet)) {
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

// getAllowedOrigins retrieves allowed origins from the DMR_ORIGINS environment variable.
// If the variable is not set it returns nil, indicating no origins are allowed.
func getAllowedOrigins() (origins []string) {
	dmrOrigins := os.Getenv("DMR_ORIGINS")
	if dmrOrigins == "" {
		return nil
	}

	for _, o := range strings.Split(dmrOrigins, ",") {
		if trimmed := strings.TrimSpace(o); trimmed != "" {
			origins = append(origins, trimmed)
		}
	}

	if len(origins) == 0 {
		return nil
	}

	return origins
}
