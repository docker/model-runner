package middleware

import (
	"net/http"
	"os"
	"strings"
)

// CorsMiddleware handles CORS and OPTIONS preflight requests with optional allowedOrigins.
// If allowedOrigins is nil or empty, it falls back to getAllowedOrigins().
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
		if origin := r.Header.Get("Origin"); origin != "" && (allowAll || originAllowed(origin, allowedSet)) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}

		// Handle OPTIONS requests.
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST")
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
