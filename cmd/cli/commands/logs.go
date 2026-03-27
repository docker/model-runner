package commands

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/cmd/cli/pkg/standalone"
	"github.com/docker/model-runner/cmd/cli/pkg/types"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
	"github.com/nxadm/tail"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

// logsDockerClient defines the Docker API surface needed by the logs command.
type logsDockerClient interface {
	ContainerLogs(
		ctx context.Context,
		container string,
		options client.ContainerLogsOptions,
	) (client.ContainerLogsResult, error)
}

// logsDockerClientFactory creates the Docker client used by the logs command.
var logsDockerClientFactory = func() (logsDockerClient, error) {
	return desktop.DockerClientForContext(
		dockerCLI,
		dockerCLI.CurrentContext(),
	)
}

// logsControllerContainerFinder resolves the controller container ID used by
// the logs command.
var logsControllerContainerFinder = func(
	ctx context.Context,
	dockerClient logsDockerClient,
) (string, error) {
	apiClient, ok := dockerClient.(client.ContainerAPIClient)
	if !ok {
		return "", errors.New(
			"docker client does not support container discovery",
		)
	}
	ctrID, _, _, err := standalone.FindControllerContainer(ctx, apiClient)
	return ctrID, err
}

// logsWindowsHomeDirResolver resolves the Windows home directory when reading
// Docker Desktop logs from WSL2.
var logsWindowsHomeDirResolver = windowsHomeDirFromWSL

func newLogsCmd() *cobra.Command {
	var follow, noEngines bool
	c := &cobra.Command{
		Use:   "logs [OPTIONS]",
		Short: "Fetch the Docker Model Runner logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogs(cmd, follow, noEngines)
		},
		ValidArgsFunction: completion.NoComplete,
	}
	c.Flags().BoolVarP(&follow, "follow", "f", false, "View logs with real-time streaming")
	c.Flags().BoolVar(&noEngines, "no-engines", false, "Exclude inference engine logs from the output")
	return c
}

// runLogs executes the logs command on the current operating system.
func runLogs(cmd *cobra.Command, follow, noEngines bool) error {
	return runLogsForEnv(cmd, follow, noEngines, runtime.GOOS, isWSL())
}

// runLogsForEnv executes the logs command using the supplied OS details. This
// helper keeps the platform-specific control flow testable.
func runLogsForEnv(
	cmd *cobra.Command,
	follow, noEngines bool,
	goos string,
	wsl bool,
) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	engineKind := modelRunner.EngineKind()
	manualTargetLocal := engineKind == types.ModelRunnerEngineKindMobyManual &&
		isLocalLogsTargetURL(modelRunner.URL(""))

	var containerLogsErr error
	if shouldUseContainerLogs(engineKind, manualTargetLocal) {
		err = printControllerContainerLogs(
			cmd.Context(),
			cmd.OutOrStdout(),
			cmd.ErrOrStderr(),
			follow,
		)
		if err == nil {
			return nil
		}
		if mustUseContainerLogs(engineKind) {
			return err
		}
		containerLogsErr = err
	}

	serviceLogPath, runtimeLogPath, err := resolveFileLogPaths(
		cmd.Context(),
		homeDir,
		goos,
		wsl,
		engineKind,
		manualTargetLocal,
		containerLogsErr,
	)
	if err != nil {
		return err
	}

	if noEngines {
		err = printMergedLog(cmd.OutOrStdout(), serviceLogPath, "")
		if err != nil {
			return err
		}
	} else {
		err = printMergedLog(
			cmd.OutOrStdout(),
			serviceLogPath,
			runtimeLogPath,
		)
		if err != nil {
			return err
		}
	}

	if !follow {
		return nil
	}

	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		os.Kill,
	)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)

	// Poll mode is needed when tailing files over a mounted filesystem
	// (Windows or WSL2 accessing the Windows host via /mnt/).
	pollMode := goos == "windows" || (goos == "linux" && wsl)

	g.Go(func() error {
		t, err := tail.TailFile(
			serviceLogPath,
			tail.Config{
				Location: &tail.SeekInfo{Offset: 0, Whence: io.SeekEnd},
				Follow:   true,
				ReOpen:   true,
				Poll:     pollMode,
			},
		)
		if err != nil {
			return err
		}
		for {
			select {
			case line, ok := <-t.Lines:
				if !ok {
					return nil
				}
				cmd.Println(line.Text)
			case <-ctx.Done():
				return t.Stop()
			}
		}
	})

	if !noEngines {
		g.Go(func() error {
			t, err := tail.TailFile(
				runtimeLogPath,
				tail.Config{
					Location: &tail.SeekInfo{Offset: 0, Whence: io.SeekEnd},
					Follow:   true,
					ReOpen:   true,
					Poll:     pollMode,
				},
			)
			if err != nil {
				return err
			}

			for {
				select {
				case line, ok := <-t.Lines:
					if !ok {
						return nil
					}
					cmd.Println(line.Text)
				case <-ctx.Done():
					return t.Stop()
				}
			}
		})
	}

	return g.Wait()
}

// shouldUseContainerLogs reports whether the logs command should try the
// controller container before falling back to Desktop log files.
func shouldUseContainerLogs(
	engineKind types.ModelRunnerEngineKind,
	manualTargetLocal bool,
) bool {
	switch engineKind {
	case types.ModelRunnerEngineKindMoby,
		types.ModelRunnerEngineKindCloud:
		return true
	case types.ModelRunnerEngineKindMobyManual:
		return manualTargetLocal
	case types.ModelRunnerEngineKindDesktop:
		return false
	default:
		return false
	}
}

// mustUseContainerLogs reports whether the logs command must succeed via the
// controller container and cannot fall back to Desktop log files.
func mustUseContainerLogs(engineKind types.ModelRunnerEngineKind) bool {
	return engineKind == types.ModelRunnerEngineKindMoby ||
		engineKind == types.ModelRunnerEngineKindCloud
}

// isLocalLogsTargetURL reports whether the given runner URL clearly targets a
// local Docker-managed endpoint.
func isLocalLogsTargetURL(rawURL string) bool {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	switch strings.ToLower(parsedURL.Hostname()) {
	case "localhost",
		"127.0.0.1",
		"::1",
		"host.docker.internal",
		"model-runner.docker.internal",
		"gateway.docker.internal":
		return true
	default:
		return false
	}
}

// printControllerContainerLogs prints logs from the controller container using
// the Docker API.
func printControllerContainerLogs(
	ctx context.Context,
	stdout io.Writer,
	stderr io.Writer,
	follow bool,
) error {
	dockerClient, err := logsDockerClientFactory()
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	ctrID, err := logsControllerContainerFinder(ctx, dockerClient)
	if err != nil {
		return fmt.Errorf("unable to identify Model Runner container: %w", err)
	}
	if ctrID == "" {
		return errors.New("unable to identify Model Runner container")
	}
	log, err := dockerClient.ContainerLogs(
		ctx,
		ctrID,
		client.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     follow,
		},
	)
	if err != nil {
		return fmt.Errorf(
			"unable to query Model Runner container logs: %w",
			err,
		)
	}
	defer log.Close()

	_, err = stdcopy.StdCopy(stdout, stderr, log)
	return err
}

// resolveFileLogPaths resolves the Docker Desktop log files for the current
// platform after any Docker API fallback has been attempted.
func resolveFileLogPaths(
	ctx context.Context,
	homeDir string,
	goos string,
	wsl bool,
	engineKind types.ModelRunnerEngineKind,
	manualTargetLocal bool,
	containerLogsErr error,
) (string, string, error) {
	switch goos {
	case "darwin":
		return filepath.Join(
				homeDir,
				"Library/Containers/com.docker.docker/Data/log/host/inference.log",
			), filepath.Join(
				homeDir,
				"Library/Containers/com.docker.docker/Data/log/host/inference-llama.cpp-server.log",
			), nil
	case "windows", "linux":
		baseDir := homeDir
		if goos == "linux" {
			if !wsl {
				if engineKind == types.ModelRunnerEngineKindMobyManual &&
					manualTargetLocal &&
					containerLogsErr != nil {
					return "", "", manualLocalLogsUnavailableError(
						containerLogsErr,
					)
				}
				return "", "", fmt.Errorf(
					"log viewing on native Linux is only supported in standalone mode",
				)
			}

			// When running inside WSL2 with Docker Desktop, the log files are on
			// the Windows host filesystem mounted under /mnt/.
			winHomeDir, err := logsWindowsHomeDirResolver(ctx)
			if err != nil {
				return "", "", fmt.Errorf(
					"unable to determine Windows home directory from WSL2: %w",
					err,
				)
			}
			baseDir = winHomeDir
		}
		return filepath.Join(
				baseDir,
				"AppData/Local/Docker/log/host/inference.log",
			), filepath.Join(
				baseDir,
				"AppData/Local/Docker/log/host/inference-llama.cpp-server.log",
			), nil
	default:
		return "", "", fmt.Errorf("unsupported OS: %s", goos)
	}
}

// manualLocalLogsUnavailableError describes why MODEL_RUNNER_HOST log access
// failed on native Linux after attempting the Docker API path.
func manualLocalLogsUnavailableError(err error) error {
	return fmt.Errorf(
		"log viewing with MODEL_RUNNER_HOST on native Linux requires "+
			"access to the local Docker daemon and a discoverable "+
			"docker-model-runner container: %w",
		err,
	)
}

// isWSL reports whether the current process is running inside a WSL2 environment.
func isWSL() bool {
	_, ok := os.LookupEnv("WSL_DISTRO_NAME")
	return ok
}

// windowsHomeDirFromWSL resolves the Windows user's home directory from
// within a WSL2 environment by running "wslpath" on the USERPROFILE path
// obtained via "wslvar". This returns a Linux path like /mnt/c/Users/Name.
func windowsHomeDirFromWSL(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "wslvar", "USERPROFILE").Output()
	if err != nil {
		return "", fmt.Errorf("wslvar USERPROFILE: %w", err)
	}
	winPath := strings.TrimSpace(string(out))
	if winPath == "" {
		return "", fmt.Errorf("USERPROFILE is empty")
	}
	out, err = exec.CommandContext(ctx, "wslpath", "-u", winPath).Output()
	if err != nil {
		return "", fmt.Errorf("wslpath -u %q: %w", winPath, err)
	}
	linuxPath := strings.TrimSpace(string(out))
	if linuxPath == "" {
		return "", fmt.Errorf("wslpath returned empty path")
	}
	return linuxPath, nil
}

var timestampRe = regexp.MustCompile(`\[(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z)\].*`)

const timeFmt = "2006-01-02T15:04:05.000000000Z"

func advanceToNextTimestamp(w io.Writer, logScanner *bufio.Scanner) (time.Time, string) {
	if logScanner == nil {
		return time.Time{}, ""
	}

	for logScanner.Scan() {
		text := logScanner.Text()
		match := timestampRe.FindStringSubmatch(text)
		if len(match) == 2 {
			timestamp, err := time.Parse(timeFmt, match[1])
			if err != nil {
				fmt.Fprintln(w, text)
				continue
			}
			return timestamp, text
		} else {
			fmt.Fprintln(w, text)
		}
	}
	return time.Time{}, ""
}

func printMergedLog(w io.Writer, logPath1, logPath2 string) error {
	var logScanner1 *bufio.Scanner
	if logPath1 != "" {
		logFile1, err := os.Open(logPath1)
		if err == nil {
			defer logFile1.Close()
			logScanner1 = bufio.NewScanner(logFile1)
		}
	}

	var logScanner2 *bufio.Scanner
	if logPath2 != "" {
		logFile2, err := os.Open(logPath2)
		if err == nil {
			defer logFile2.Close()
			logScanner2 = bufio.NewScanner(logFile2)
		}
	}

	var timestamp1 time.Time
	var timestamp2 time.Time
	var line1 string
	var line2 string

	timestamp1, line1 = advanceToNextTimestamp(w, logScanner1)
	timestamp2, line2 = advanceToNextTimestamp(w, logScanner2)

	for line1 != "" && line2 != "" {
		if !timestamp2.Before(timestamp1) {
			fmt.Fprintln(w, line1)
			timestamp1, line1 = advanceToNextTimestamp(w, logScanner1)
		} else {
			fmt.Fprintln(w, line2)
			timestamp2, line2 = advanceToNextTimestamp(w, logScanner2)
		}
	}

	if line1 != "" {
		fmt.Fprintln(w, line1)
		for logScanner1.Scan() {
			fmt.Fprintln(w, logScanner1.Text())
		}
	}
	if line2 != "" {
		fmt.Fprintln(w, line2)
		for logScanner2.Scan() {
			fmt.Fprintln(w, logScanner2.Text())
		}
	}

	return nil
}
