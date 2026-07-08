package llamacpp

import (
	"context"
	"net/http"
	"path/filepath"

	"github.com/docker/model-runner/pkg/logging"
)

func (l *llamaCpp) ensureLatestLlamaCpp(_ context.Context, log logging.Logger, _ *http.Client) error {
	// On Linux the binary is bundled into the container image at installDir;
	// there is no on-demand download.
	l.setRunningStatus(log, filepath.Join(l.installDir, resolveLlamaServerBin(l.installDir)), "", "")
	return errLlamaCppUpdateDisabled
}
