package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCorsMiddleware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		allowedOrigins []string
		method         string
		origin         string
		wantStatus     int
		wantHeaders    map[string]string
	}{
		{
			name:           "AllowAll",
			allowedOrigins: []string{"*"},
			method:         "GET",
			origin:         "http://example.com",
			wantStatus:     http.StatusOK,
			wantHeaders:    map[string]string{"Access-Control-Allow-Origin": "http://example.com"},
		},
		{
			name:           "AllowSpecificOrigin",
			allowedOrigins: []string{"http://foo.com"},
			method:         "GET",
			origin:         "http://foo.com",
			wantStatus:     http.StatusOK,
			wantHeaders:    map[string]string{"Access-Control-Allow-Origin": "http://foo.com"},
		},
		{
			name:           "DisallowOrigin",
			allowedOrigins: []string{"http://foo.com"},
			method:         "GET",
			origin:         "http://bar.com",
			wantStatus:     http.StatusForbidden,
			wantHeaders:    map[string]string{"Access-Control-Allow-Origin": ""},
		},
		{
			name:           "OptionsRequest",
			allowedOrigins: []string{"http://foo.com"},
			method:         "OPTIONS",
			origin:         "http://foo.com",
			wantStatus:     http.StatusNoContent,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Credentials": "true",
				"Access-Control-Allow-Methods":     "GET, POST, DELETE",
				"Access-Control-Allow-Headers":     "*",
			},
		},
		{
			name:           "DeleteRequest",
			allowedOrigins: []string{"http://foo.com"},
			method:         "DELETE",
			origin:         "http://foo.com",
			wantStatus:     http.StatusOK,
			wantHeaders:    map[string]string{"Access-Control-Allow-Origin": "http://foo.com"},
		},
		{
			name:           "NoOriginHeader",
			allowedOrigins: []string{"http://foo.com"},
			method:         "GET",
			origin:         "",
			wantStatus:     http.StatusOK,
			wantHeaders:    map[string]string{"Access-Control-Allow-Origin": ""},
		},
		{
			name:           "DisableAllOrigins",
			allowedOrigins: nil,
			method:         "GET",
			origin:         "http://foo.com",
			wantStatus:     http.StatusForbidden,
			wantHeaders:    map[string]string{"Access-Control-Allow-Origin": ""},
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			handler := CorsMiddleware(tt.allowedOrigins, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequest(tt.method, "/", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rec.Code)
			}
			for k, v := range tt.wantHeaders {
				if got := rec.Header().Get(k); got != v {
					t.Errorf("expected %s to be %q, got %q", k, v, got)
				}
			}
		})
	}
}

func TestOriginAllowed(t *testing.T) {
	t.Parallel()
	set := map[string]struct{}{
		"http://foo.com": {},
	}
	if !originAllowed("http://foo.com", set) {
		t.Errorf("expected originAllowed to return true")
	}
	if originAllowed("http://bar.com", set) {
		t.Errorf("expected originAllowed to return false")
	}
}
