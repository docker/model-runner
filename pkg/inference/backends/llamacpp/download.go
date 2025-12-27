package llamacpp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/docker/model-runner/pkg/internal/dockerhub"
	"github.com/docker/model-runner/pkg/logging"
)

//nolint:unused // Used in platform-specific files (download_darwin.go, download_windows.go)
const (
	hubNamespace = "docker"
	hubRepo      = "docker-model-backend-llamacpp"
)

var (
	ShouldUseGPUVariant       bool
	ShouldUseGPUVariantLock   sync.Mutex
	ShouldUpdateServer        = true
	ShouldUpdateServerLock    sync.Mutex
	DesiredServerVersion      = "latest"
	DesiredServerVersionLock  sync.Mutex
	errLlamaCppUpToDate       = errors.New("bundled llama.cpp version is up to date, no need to update")
	errLlamaCppUpdateDisabled = errors.New("llama.cpp auto-updated is disabled")
)

func GetDesiredServerVersion() string {
	DesiredServerVersionLock.Lock()
	defer DesiredServerVersionLock.Unlock()
	return DesiredServerVersion
}

func SetDesiredServerVersion(version string) {
	DesiredServerVersionLock.Lock()
	defer DesiredServerVersionLock.Unlock()
	DesiredServerVersion = version
}

//nolint:unused // Used in platform-specific files (download_darwin.go, download_windows.go)
func (l *llamaCpp) downloadLatestLlamaCpp(ctx context.Context, log logging.Logger, httpClient *http.Client,
	llamaCppPath, vendoredServerStoragePath, desiredVersion, desiredVariant string,
) error {
	ShouldUpdateServerLock.Lock()
	shouldUpdateServer := ShouldUpdateServer
	ShouldUpdateServerLock.Unlock()
	if !shouldUpdateServer {
		log.Infof("downloadLatestLlamaCpp: update disabled")
		return errLlamaCppUpdateDisabled
	}

	log.Infof("downloadLatestLlamaCpp: %s, %s, %s, %s", desiredVersion, desiredVariant, vendoredServerStoragePath, llamaCppPath)
	desiredTag := desiredVersion + "-" + desiredVariant
	url := fmt.Sprintf("https://hub.docker.com/v2/namespaces/%s/repositories/%s/tags/%s", hubNamespace, hubRepo, desiredTag)
	resp, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// https://docs.docker.com/reference/api/hub/latest/#tag/repositories/paths/~1v2~1namespaces~1%7Bnamespace%7D~1repositories~1%7Brepository%7D~1tags~1%7Btag%7D/get
	var response struct {
		Name   string `json:"name"`
		Digest string `json:"digest"`
	}

	if unmarshalErr := json.Unmarshal(body, &response); unmarshalErr != nil {
		return fmt.Errorf("failed to unmarshal response body: %w", unmarshalErr)
	}

	var latest string
	if response.Name == desiredTag {
		latest = response.Digest
	}
	if latest == "" {
		log.Warnf("could not fing the %s tag, hub response: %s", desiredTag, body)
		return fmt.Errorf("could not find the %s tag", desiredTag)
	}

	bundledVersionFile := filepath.Join(vendoredServerStoragePath, "com.docker.llama-server.digest")
	currentVersionFile := filepath.Join(filepath.Dir(llamaCppPath), ".llamacpp_version")

	data, err := os.ReadFile(bundledVersionFile)
	if err != nil {
		return fmt.Errorf("failed to read bundled llama.cpp version: %w", err)
	} else if strings.TrimSpace(string(data)) == latest {
		l.status = fmt.Sprintf("running llama.cpp %s (%s) version: %s",
			desiredTag, latest, getLlamaCppVersion(log, filepath.Join(vendoredServerStoragePath, "com.docker.llama-server")))
		return errLlamaCppUpToDate
	}

	data, err = os.ReadFile(currentVersionFile)
	if err != nil {
		log.Warnf("failed to read current llama.cpp version: %v", err)
		log.Warnf("proceeding to update llama.cpp binary")
	} else if strings.TrimSpace(string(data)) == latest {
		log.Infoln("current llama.cpp version is already up to date")
		if _, statErr := os.Stat(llamaCppPath); statErr == nil {
			l.status = fmt.Sprintf("running llama.cpp %s (%s) version: %s",
				desiredTag, latest, getLlamaCppVersion(log, llamaCppPath))
			return nil
		}
		log.Infoln("llama.cpp binary must be updated, proceeding to update it")
	} else {
		log.Infof("current llama.cpp version is outdated: %s vs %s, proceeding to update it", strings.TrimSpace(string(data)), latest)
	}

	image := fmt.Sprintf("registry-1.docker.io/%s/%s@%s", hubNamespace, hubRepo, latest)
	downloadDir, err := os.MkdirTemp("", "llamacpp-install")
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(downloadDir)

	l.status = fmt.Sprintf("downloading %s (%s) variant of llama.cpp", desiredTag, latest)
	if extractErr := extractFromImage(ctx, log, image, runtime.GOOS, runtime.GOARCH, downloadDir); extractErr != nil {
		return fmt.Errorf("could not extract image: %w", extractErr)
	}

	if err := os.RemoveAll(filepath.Dir(llamaCppPath)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to clear inference binary dir: %w", err)
	}
	if err := os.RemoveAll(filepath.Join(filepath.Dir(filepath.Dir(llamaCppPath)), "lib")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to clear inference library dir: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(filepath.Dir(llamaCppPath)), 0o755); err != nil {
		return fmt.Errorf("could not create directory for llama.cpp artifacts: %w", err)
	}

	rootDir := fmt.Sprintf("com.docker.llama-server.native.%s.%s.%s", runtime.GOOS, desiredVariant, runtime.GOARCH)
	if err := os.Rename(filepath.Join(downloadDir, rootDir, "bin"), filepath.Dir(llamaCppPath)); err != nil {
		return fmt.Errorf("could not move llama.cpp binary: %w", err)
	}
	if err := os.Chmod(llamaCppPath, 0o755); err != nil {
		return fmt.Errorf("could not chmod llama.cpp binary: %w", err)
	}

	libDir := filepath.Join(downloadDir, rootDir, "lib")
	fi, err := os.Stat(libDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to stat llama.cpp lib dir: %w", err)
	}
	if err == nil && fi.IsDir() {
		if err := os.Rename(libDir, filepath.Join(filepath.Dir(filepath.Dir(llamaCppPath)), "lib")); err != nil {
			return fmt.Errorf("could not move llama.cpp libs: %w", err)
		}
	}

	log.Infoln("successfully updated llama.cpp binary")
	l.status = fmt.Sprintf("running llama.cpp %s (%s) version: %s", desiredTag, latest, getLlamaCppVersion(log, llamaCppPath))
	log.Infoln(l.status)

	if err := os.WriteFile(currentVersionFile, []byte(latest), 0o644); err != nil {
		log.Warnf("failed to save llama.cpp version: %v", err)
	}

	return nil
}

//nolint:unused // Used in platform-specific files (download_darwin.go, download_windows.go)
func extractFromImage(ctx context.Context, log logging.Logger, image, requiredOs, requiredArch, destination string) error {
	log.Infof("Extracting image %q to %q", image, destination)
	tmpDir, err := os.MkdirTemp("", "docker-tar-extract")
	if err != nil {
		return err
	}
	imageTar := filepath.Join(tmpDir, "save.tar")
	if err := dockerhub.PullPlatform(ctx, image, imageTar, requiredOs, requiredArch); err != nil {
		return err
	}
	return dockerhub.Extract(imageTar, requiredArch, requiredOs, destination)
}

func getLlamaCppVersion(log logging.Logger, llamaCpp string) string {
	output, err := exec.Command(llamaCpp, "--version").CombinedOutput()
	if err != nil {
		log.Warnf("could not get llama.cpp version: %v", err)
		return "unknown"
	}
	re := regexp.MustCompile(`version: \d+ \((\w+)\)`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) == 2 {
		return matches[1]
	}
	log.Warnf("failed to parse llama.cpp version from output:\n%s", strings.TrimSpace(string(output)))
	return "unknown"
}
