package models_test

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/logging"
)

// TestCreateModelSSRF_RealmNotFollowedToInternalService drives the pull from the
// unauthenticated HTTP surface a caller actually reaches — POST /models/create —
// all the way down through the manager, distribution client, and containerd
// resolver. A malicious registry answers every request with a 401 Bearer
// challenge whose realm points at a loopback "internal service". The registry
// itself must be contacted (proving the request reached the pull path), but the
// realm must never be followed, so the internal service receives nothing.
func TestCreateModelSSRF_RealmNotFollowedToInternalService(t *testing.T) {
	var internalHits, registryHits atomic.Int32

	internalService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		internalHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"token":"leaked-via-ssrf"}`)
	}))
	defer internalService.Close()

	maliciousRegistry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		registryHits.Add(1)
		w.Header().Set("WWW-Authenticate",
			fmt.Sprintf(`Bearer realm="%s/token",service="evil-registry"`, internalService.URL))
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer maliciousRegistry.Close()

	log := logging.NewLogger(slog.LevelError)
	manager := models.NewManager(log, models.ClientConfig{
		StoreRootPath: t.TempDir(),
		Logger:        log,
		UserAgent:     "model-runner-test",
		PlainHTTP:     true,
	})
	apiServer := httptest.NewServer(models.NewHTTPHandler(log, manager, nil))
	defer apiServer.Close()

	registryHost := strings.TrimPrefix(maliciousRegistry.URL, "http://")
	body := fmt.Sprintf(`{"from":%q}`, registryHost+"/evil/model:latest")
	resp, err := http.Post(apiServer.URL+"/models/create", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /models/create: %v", err)
	}
	resp.Body.Close()

	if got := registryHits.Load(); got == 0 {
		t.Fatalf("test is inconclusive: the malicious registry was never contacted, so the pull path was not exercised")
	}
	if got := internalHits.Load(); got != 0 {
		t.Errorf("SSRF not blocked end to end: the internal service at %s was contacted %d time(s) via the token realm advertised by the registry", internalService.URL, got)
	}
}
