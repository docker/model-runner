package scheduling

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/sirupsen/logrus"
)

type systemMemoryInfo struct{}

func (i systemMemoryInfo) HaveSufficientMemory(req inference.RequiredMemory) (bool, error) {
	return true, nil
}

func (i systemMemoryInfo) GetTotalMemory() inference.RequiredMemory {
	return inference.RequiredMemory{}
}

func TestCors(t *testing.T) {
	// Verify that preflight requests work against non-existing handlers or
	// method-specific handlers that do not support OPTIONS
	t.Parallel()
	tests := []struct {
		name string
		path string
	}{
		{
			name: "root",
			path: "/",
		},
		{
			name: "status",
			path: "/status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			discard := logrus.New()
			discard.SetOutput(io.Discard)
			log := logrus.NewEntry(discard)
			s := NewScheduler(log, nil, nil, nil, nil, nil, systemMemoryInfo{})
			httpHandler := NewHTTPHandler(s, nil, []string{"*"})
			req := httptest.NewRequest(http.MethodOptions, "http://model-runner.docker.internal"+tt.path, http.NoBody)
			req.Header.Set("Origin", "docker.com")
			w := httptest.NewRecorder()
			httpHandler.ServeHTTP(w, req)

			if w.Code != http.StatusNoContent {
				t.Errorf("Expected status code 204 for OPTIONS request, got %d", w.Code)
			}
		})
	}
}
