package llamacpp

import (
	"context"
	"fmt"
	"net/http"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/logging"
)

func (l *llamaCpp) ensureLatestLlamaCpp(ctx context.Context, log logging.Logger, httpClient *http.Client,
	llamaCppPath, vendoredServerStoragePath string,
) error {
	desiredVersion := GetDesiredServerVersion()
	desiredVariant := "cpu"
	l.status = inference.FormatInstalling(fmt.Sprintf("%s llama.cpp %s", inference.DetailCheckingForUpdates, desiredVariant))
	return l.downloadLatestLlamaCpp(ctx, log, httpClient, llamaCppPath, vendoredServerStoragePath, desiredVersion,
		desiredVariant)
}
