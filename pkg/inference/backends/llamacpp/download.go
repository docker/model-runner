package llamacpp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/internal/dockerhub"
	"github.com/docker/model-runner/pkg/logging"
)

//nolint:unused // Used in platform-specific files (download_darwin.go, download_windows.go)
const (
	hubNamespace = "docker"
	hubRepo      = "docker-model-backend-llamacpp"

	// LatestServerVersion is the sentinel that opts into tracking the mutable
	// "latest" tag rather than a pinned release.
	LatestServerVersion = "latest"

	// pinnedServerVersion is the default llama.cpp version this model-runner
	// build downloads on macOS/Windows. It is a tag of the
	// docker/docker-model-backend-llamacpp image and must be bumped together
	// with the Linux bundle (Dockerfile LLAMA_SERVER_VERSION) whenever llama.cpp
	// is upgraded, so all platforms ship a consistent, tested build. Users can
	// override it via LLAMA_SERVER_VERSION or `--llama-server-version`.
	pinnedServerVersion = "v0.0.34"
)

var (
	ShouldUseGPUVariant       bool
	ShouldUseGPUVariantLock   sync.Mutex
	ShouldUpdateServer        = true
	ShouldUpdateServerLock    sync.Mutex
	DesiredServerVersion      = pinnedServerVersion
	DesiredServerVersionLock  sync.Mutex
	errLlamaCppUpToDate       = errors.New("llama.cpp version is up to date, no need to update")
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
func (l *llamaCpp) downloadLatestLlamaCpp(ctx context.Context, log logging.Logger,
	desiredVersion, desiredVariant string,
) error {
	llamaCppPath := filepath.Join(l.installDir, l.downloadBinaryName())
	desiredTag := desiredVersion + "-" + desiredVariant
	binaryPresent := false
	if _, statErr := os.Stat(llamaCppPath); statErr == nil {
		binaryPresent = true
	}
	rec := l.readInstalledVersion()

	ShouldUpdateServerLock.Lock()
	shouldUpdateServer := ShouldUpdateServer
	ShouldUpdateServerLock.Unlock()
	if !shouldUpdateServer {
		log.Info("downloadLatestLlamaCpp: update disabled")
		if binaryPresent {
			l.setRunningStatus(log, llamaCppPath, desiredTag, rec.Digest)
		}
		return errLlamaCppUpdateDisabled
	}

	log.Info("downloadLatestLlamaCpp", "desiredVersion", desiredVersion, "desiredVariant", desiredVariant, "installDir", l.installDir)

	// Fast path: a pinned (immutable) version tag that is already installed
	// needs no registry round-trip at all, so startup works offline. Only the
	// mutable "latest" tag must always be re-resolved to pick up new pushes.
	if binaryPresent && desiredVersion != LatestServerVersion && rec.Tag == desiredTag {
		log.Info("pinned llama.cpp version already installed, skipping update check", "tag", desiredTag)
		l.setRunningStatus(log, llamaCppPath, desiredTag, rec.Digest)
		return errLlamaCppUpToDate
	}

	// Resolve the desired tag to a digest via the Registry HTTP API v2. This
	// honors l.registryMirrors (typically a corporate Artifactory / Nexus /
	// Harbor mirror configured for docker.io) and credentials populated by
	// `docker login`, so customers behind a private mirror with no direct
	// egress to registry-1.docker.io can still resolve and pull the backend
	// image. See docker/model-runner#TBD.
	tagRef := fmt.Sprintf("registry-1.docker.io/%s/%s:%s", hubNamespace, hubRepo, desiredTag)
	latest, err := dockerhub.ResolveDigest(ctx, tagRef, l.registryMirrors)
	if err != nil {
		log.Warn("could not resolve llama.cpp tag", "tag", desiredTag, "mirrors", l.registryMirrors, "error", err)
		return fmt.Errorf("could not resolve the %s tag: %w", desiredTag, err)
	}

	// If we have already downloaded this exact digest and the binary is still
	// present, there is nothing to do. Unlike the previous Docker Desktop
	// bundled model, there is no vendored binary to compare against here.
	if binaryPresent && rec.Digest == latest {
		log.Info("current llama.cpp version is already up to date")
		l.setRunningStatus(log, llamaCppPath, desiredTag, latest)
		return errLlamaCppUpToDate
	}
	if rec.Digest != "" && rec.Digest != latest {
		log.Info("current llama.cpp version is outdated, proceeding to update", "current", rec.Digest, "latest", latest)
	}

	image := fmt.Sprintf("registry-1.docker.io/%s/%s@%s", hubNamespace, hubRepo, latest)
	downloadDir, err := os.MkdirTemp("", "llamacpp-install")
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(downloadDir)

	l.status = inference.FormatInstalling(fmt.Sprintf("%s llama.cpp %s", inference.DetailDownloading, desiredTag))
	if extractErr := extractFromImage(ctx, log, image, runtime.GOOS, runtime.GOARCH, downloadDir, l.registryMirrors); extractErr != nil {
		return fmt.Errorf("could not extract image: %w", extractErr)
	}

	libDir := filepath.Join(filepath.Dir(l.installDir), "lib")
	if err := os.RemoveAll(l.installDir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to clear inference binary dir: %w", err)
	}
	if err := os.RemoveAll(libDir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to clear inference library dir: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(l.installDir), 0o755); err != nil {
		return fmt.Errorf("could not create directory for llama.cpp artifacts: %w", err)
	}

	rootDir := fmt.Sprintf("com.docker.llama-server.native.%s.%s.%s", runtime.GOOS, desiredVariant, runtime.GOARCH)
	if err := os.Rename(filepath.Join(downloadDir, rootDir, "bin"), l.installDir); err != nil {
		return fmt.Errorf("could not move llama.cpp binary: %w", err)
	}
	if err := os.Chmod(llamaCppPath, 0o755); err != nil {
		return fmt.Errorf("could not chmod llama.cpp binary: %w", err)
	}

	srcLibDir := filepath.Join(downloadDir, rootDir, "lib")
	fi, err := os.Stat(srcLibDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to stat llama.cpp lib dir: %w", err)
	}
	if err == nil && fi.IsDir() {
		if err := os.Rename(srcLibDir, libDir); err != nil {
			return fmt.Errorf("could not move llama.cpp libs: %w", err)
		}
	}

	log.Info("successfully updated llama.cpp binary")
	l.setRunningStatus(log, llamaCppPath, desiredTag, latest)
	log.Info(l.status)

	l.writeInstalledVersion(log, installedVersion{Tag: desiredTag, Digest: latest})

	return nil
}

// installedVersion records which llama.cpp image tag and digest are currently
// installed in installDir. Tracking the tag (not just the digest) lets us skip
// the registry round-trip for immutable pinned versions.
//
//nolint:unused // Used in platform-specific files (download_darwin.go, download_windows.go)
type installedVersion struct {
	Tag    string `json:"tag"`
	Digest string `json:"digest"`
}

//nolint:unused // Used in platform-specific files (download_darwin.go, download_windows.go)
func (l *llamaCpp) versionFilePath() string {
	return filepath.Join(l.installDir, ".llamacpp_version")
}

// readInstalledVersion reads the recorded install metadata, tolerating the
// legacy format where the file held only the raw digest.
//
//nolint:unused // Used in platform-specific files (download_darwin.go, download_windows.go)
func (l *llamaCpp) readInstalledVersion() installedVersion {
	data, err := os.ReadFile(l.versionFilePath())
	if err != nil {
		return installedVersion{}
	}
	var rec installedVersion
	if json.Unmarshal(data, &rec) == nil && rec.Digest != "" {
		return rec
	}
	// Legacy format: the file previously held only the digest string.
	return installedVersion{Digest: strings.TrimSpace(string(data))}
}

//nolint:unused // Used in platform-specific files (download_darwin.go, download_windows.go)
func (l *llamaCpp) writeInstalledVersion(log logging.Logger, rec installedVersion) {
	data, err := json.Marshal(rec)
	if err != nil {
		log.Warn("failed to marshal llama.cpp version", "error", err)
		return
	}
	if err := os.WriteFile(l.versionFilePath(), data, 0o644); err != nil {
		log.Warn("failed to save llama.cpp version", "error", err)
	}
}

//nolint:unused // Used in platform-specific files (download_darwin.go, download_windows.go)
func extractFromImage(ctx context.Context, log logging.Logger, image, requiredOs, requiredArch, destination string, mirrors []string) error {
	log.Info("Extracting image", "image", image, "destination", destination)
	tmpDir, err := os.MkdirTemp("", "docker-tar-extract")
	if err != nil {
		return err
	}
	imageTar := filepath.Join(tmpDir, "save.tar")
	if err := dockerhub.PullPlatform(ctx, image, imageTar, requiredOs, requiredArch, mirrors); err != nil {
		return err
	}
	return dockerhub.Extract(imageTar, requiredArch, requiredOs, destination)
}

func (l *llamaCpp) setRunningStatus(log logging.Logger, binaryPath, variant, digest string) {
	version := getLlamaCppVersion(log, binaryPath)
	if variant == "" && digest == "" {
		l.status = inference.FormatRunning(fmt.Sprintf("llama.cpp %s", version))
	} else {
		l.status = inference.FormatRunning(fmt.Sprintf("llama.cpp %s (%s) %s", variant, digest, version))
	}
}

func getLlamaCppVersion(log logging.Logger, llamaCpp string) string {
	output, err := exec.Command(llamaCpp, "--version").CombinedOutput()
	if err != nil {
		log.Warn("could not get llama.cpp version", "error", err)
		return "unknown"
	}
	re := regexp.MustCompile(`version: \d+ \((\w+)\)`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) == 2 {
		return matches[1]
	}
	log.Warn("failed to parse llama.cpp version from output", "output", strings.TrimSpace(string(output)))
	return "unknown"
}
