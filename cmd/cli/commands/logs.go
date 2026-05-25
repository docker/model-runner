package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/cmd/cli/pkg/standalone"
	"github.com/docker/model-runner/cmd/cli/pkg/types"
	dmrlogs "github.com/docker/model-runner/pkg/logs"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
	"github.com/spf13/cobra"
)

func newLogsCmd() *cobra.Command {
	var follow, noEngines bool
	c := &cobra.Command{
		Use:   "logs [OPTIONS]",
		Short: "Fetch the Docker Model Runner logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Standalone mode: fetch container logs via Docker API.
			engineKind := modelRunner.EngineKind()
			useStandaloneLogs := engineKind == types.ModelRunnerEngineKindMoby ||
				engineKind == types.ModelRunnerEngineKindCloud
			if useStandaloneLogs {
				dockerClient, err := desktop.DockerClientForContext(
					dockerCLI, dockerCLI.CurrentContext(),
				)
				if err != nil {
					return fmt.Errorf("failed to create Docker client: %w", err)
				}
				ctrID, _, _, err := standalone.FindControllerContainer(
					cmd.Context(), dockerClient,
				)
				if err != nil {
					return fmt.Errorf(
						"unable to identify Model Runner container: %w", err,
					)
				} else if ctrID == "" {
					return errors.New("unable to identify Model Runner container")
				}
				log, err := dockerClient.ContainerLogs(
					cmd.Context(), ctrID, client.ContainerLogsOptions{
						ShowStdout: true,
						ShowStderr: true,
						Follow:     follow,
					},
				)
				if err != nil {
					return fmt.Errorf(
						"unable to query Model Runner container logs: %w", err,
					)
				}
				defer log.Close()
				_, err = stdcopy.StdCopy(os.Stdout, os.Stderr, log)
				return err
			}

			// Desktop mode: try local log files first.
			serviceLogPath, runtimeLogPath, localErr := resolveDesktopLogPaths(
				cmd.Context(),
			)
			if localErr == nil {
				// Verify we can actually open the service log.
				f, openErr := os.Open(serviceLogPath)
				if openErr != nil {
					localErr = openErr
				} else {
					f.Close()
				}
			}

			if localErr != nil {
				// Local files unavailable (e.g. running inside a container).
				// Fall back to the DMR /logs API.
				apiErr := desktopClient.Logs(
					cmd.Context(), follow, noEngines, cmd.OutOrStdout(),
				)
				if apiErr != nil {
					return fmt.Errorf(
						"local logs unavailable (%w); API fallback failed: %w",
						localErr, apiErr,
					)
				}
				return nil
			}

			// Local files are accessible: use shared merge/follow logic.
			enginePath := ""
			if !noEngines {
				enginePath = runtimeLogPath
			}

			// Poll mode is needed when tailing files over a mounted
			// filesystem (Windows or WSL2 accessing the Windows host).
			pollMode := runtime.GOOS == "windows" ||
				(runtime.GOOS == "linux" && isWSL())

			result, err := dmrlogs.MergeLogs(
				cmd.OutOrStdout(), serviceLogPath, enginePath,
			)
			if err != nil {
				return err
			}
			if !follow {
				return nil
			}
			return dmrlogs.Follow(
				cmd.Context(), cmd.OutOrStdout(),
				serviceLogPath, enginePath,
				result, pollMode,
			)
		},
		ValidArgsFunction: completion.NoComplete,
	}
	c.Flags().BoolVarP(&follow, "follow", "f", false, "View logs with real-time streaming")
	c.Flags().BoolVar(&noEngines, "no-engines", false, "Exclude inference engine logs from the output")
	return c
}

// resolveDesktopLogPaths returns the service and engine log file paths
// for Docker Desktop mode. It returns an error when the OS is not
// supported or path discovery fails (e.g. native Linux, WSL failure).
func resolveDesktopLogPaths(ctx context.Context) (string, string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("home directory: %w", err)
	}

	switch runtime.GOOS {
	case "darwin":
		base := filepath.Join(
			homeDir,
			"Library/Containers/com.docker.docker/Data/log/host",
		)
		return filepath.Join(base, dmrlogs.ServiceLogName),
			filepath.Join(base, dmrlogs.EngineLogName),
			nil
	case "windows":
		base := filepath.Join(homeDir, "AppData/Local/Docker/log/host")
		return filepath.Join(base, dmrlogs.ServiceLogName),
			filepath.Join(base, dmrlogs.EngineLogName),
			nil
	case "linux":
		if !isWSL() {
			return "", "", fmt.Errorf(
				"log viewing on native Linux is only supported in standalone mode",
			)
		}
		// In WSL2 with Docker Desktop, log files are on the Windows
		// host filesystem mounted under /mnt/.
		winHomeDir, wslErr := windowsHomeDirFromWSL(ctx)
		if wslErr != nil {
			return "", "", fmt.Errorf(
				"unable to determine Windows home directory from WSL2: %w",
				wslErr,
			)
		}
		base := filepath.Join(winHomeDir, "AppData/Local/Docker/log/host")
		return filepath.Join(base, dmrlogs.ServiceLogName),
			filepath.Join(base, dmrlogs.EngineLogName),
			nil
	default:
		return "", "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// isWSL reports whether the current process is running inside a WSL2
// environment.
func isWSL() bool {
	_, ok := os.LookupEnv("WSL_DISTRO_NAME")
	return ok
}

// windowsHomeDirFromWSL resolves the Windows user's home directory
// from within a WSL2 environment by running "wslpath" on the
// USERPROFILE path obtained via "wslvar". Returns a Linux path such
// as /mnt/c/Users/Name.
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
