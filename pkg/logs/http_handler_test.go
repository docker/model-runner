package logs

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHTTPHandlerStreamsContent verifies that the handler returns the
// merged log content.
func TestHTTPHandlerStreamsContent(t *testing.T) {
	dir := t.TempDir()
	writeLog(t, filepath.Join(dir, ServiceLogName),
		"[2026-03-09T12:00:00.000000000Z] service line\n")
	writeLog(t, filepath.Join(dir, EngineLogName),
		"[2026-03-09T12:00:01.000000000Z] engine line\n")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/logs", http.NoBody)
	NewHTTPHandler(dir)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "service line") {
		t.Errorf("service line missing from response: %q", body)
	}
	if !strings.Contains(body, "engine line") {
		t.Errorf("engine line missing from response: %q", body)
	}
}

// TestHTTPHandlerNoEngines verifies that the no-engines parameter
// excludes the engine log.
func TestHTTPHandlerNoEngines(t *testing.T) {
	dir := t.TempDir()
	writeLog(t, filepath.Join(dir, ServiceLogName),
		"[2026-03-09T12:00:00.000000000Z] service line\n")
	writeLog(t, filepath.Join(dir, EngineLogName),
		"[2026-03-09T12:00:01.000000000Z] engine line\n")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/logs?no-engines=true", http.NoBody)
	NewHTTPHandler(dir)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "service line") {
		t.Errorf("service line missing from response: %q", body)
	}
	if strings.Contains(body, "engine line") {
		t.Errorf("engine line should be absent from response: %q", body)
	}
}

// TestHTTPHandlerServiceLogMissing verifies that 404 is returned
// when the service log does not exist.
func TestHTTPHandlerServiceLogMissing(t *testing.T) {
	dir := t.TempDir()
	// No service log created.

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/logs", http.NoBody)
	NewHTTPHandler(dir)(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestHTTPHandlerMissingEngineLogTolerated verifies that a missing
// engine log is tolerated when no-engines is false.
func TestHTTPHandlerMissingEngineLogTolerated(t *testing.T) {
	dir := t.TempDir()
	writeLog(t, filepath.Join(dir, ServiceLogName),
		"[2026-03-09T12:00:00.000000000Z] service line\n")
	// No engine log created.

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/logs", http.NoBody)
	NewHTTPHandler(dir)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "service line") {
		t.Errorf("service line missing from response: %q", rec.Body.String())
	}
}

// TestHTTPHandlerLogDirEmpty verifies that 501 is returned when no
// log directory is configured.
func TestHTTPHandlerLogDirEmpty(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/logs", http.NoBody)
	NewHTTPHandler("")(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

// TestHTTPHandlerInvalidQueryParam verifies that 400 is returned for
// an unparseable boolean query parameter.
func TestHTTPHandlerInvalidQueryParam(t *testing.T) {
	dir := t.TempDir()
	writeLog(t, filepath.Join(dir, ServiceLogName), "line\n")

	for _, tc := range []struct{ param, value string }{
		{"follow", "notabool"},
		{"no-engines", "notabool"},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(
			http.MethodGet, "/logs?"+tc.param+"="+tc.value, http.NoBody,
		)
		NewHTTPHandler(dir)(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf(
				"param %q=%q: status = %d, want %d",
				tc.param, tc.value, rec.Code, http.StatusBadRequest,
			)
		}
	}
}

// TestHTTPHandlerContentType verifies that text/plain is set.
func TestHTTPHandlerContentType(t *testing.T) {
	dir := t.TempDir()
	writeLog(t, filepath.Join(dir, ServiceLogName), "line\n")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/logs", http.NoBody)
	NewHTTPHandler(dir)(rec, req)

	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
}

// writeLog is a test helper that writes content to path.
func writeLog(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
