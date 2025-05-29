package llamacpp

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/docker/model-runner/pkg/logging"
)

func (l *llamaCpp) ensureLatestLlamaCpp(_ context.Context, log logging.Logger, _ *http.Client,
	_, vendoredServerStoragePath string,
) error {
	l.status = fmt.Sprintf("running llama.cpp version: %s",
		getLlamaCppVersion(log, filepath.Join(vendoredServerStoragePath, "com.docker.llama-server")))
	return errLlamaCppUpdateDisabled
}
