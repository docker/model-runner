package llamacpp

import (
	"context"
	"net/http"

	"github.com/docker/model-runner/pkg/logging"
)

func (l *llamaCpp) ensureLatestLlamaCpp(ctx context.Context, log logging.Logger, _ *http.Client) error {
	desiredVersion := GetDesiredServerVersion()
	desiredVariant := "metal"
	return l.downloadLatestLlamaCpp(ctx, log, desiredVersion, desiredVariant)
}
