package llamacpp

import (
	"context"
	"net/http"
	"path/filepath"

	"github.com/docker/model-runner/pkg/logging"
)

func (l *llamaCpp) ensureLatestLlamaCpp(_ context.Context, log logging.Logger, _ *http.Client) error {
	l.setRunningStatus(log, filepath.Join(l.installDir, "com.docker.llama-server"), "", "")
	return errLlamaCppUpdateDisabled
}
