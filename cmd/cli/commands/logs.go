package commands

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
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

func newLogsCmd() *cobra.Command {
	var follow, noEngines bool
	c := &cobra.Command{
		Use:   "logs [OPTIONS]",
		Short: "Fetch the Docker Model Runner logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return err
			}

			// If we're running in standalone mode, then print the container
			// logs.
			engineKind := modelRunner.EngineKind()
			useStandaloneLogs := engineKind == types.ModelRunnerEngineKindMoby ||
				engineKind == types.ModelRunnerEngineKindCloud
			if useStandaloneLogs {
				dockerClient, err := desktop.DockerClientForContext(dockerCLI, dockerCLI.CurrentContext())
				if err != nil {
					return fmt.Errorf("failed to create Docker client: %w", err)
				}
				ctrID, _, _, err := standalone.FindControllerContainer(cmd.Context(), dockerClient)
				if err != nil {
					return fmt.Errorf("unable to identify Model Runner container: %w", err)
				} else if ctrID == "" {
					return errors.New("unable to identify Model Runner container")
				}
				log, err := dockerClient.ContainerLogs(cmd.Context(), ctrID, client.ContainerLogsOptions{
					ShowStdout: true,
					ShowStderr: true,
					Follow:     follow,
				})
				if err != nil {
					return fmt.Errorf("unable to query Model Runner container logs: %w", err)
				}
				defer log.Close()
				_, err = stdcopy.StdCopy(os.Stdout, os.Stderr, log)
				return err
			}

			var serviceLogPath string
			var runtimeLogPath string
			switch runtime.GOOS {
			case "darwin":
				serviceLogPath = filepath.Join(homeDir, "Library/Containers/com.docker.docker/Data/log/host/inference.log")
				runtimeLogPath = filepath.Join(homeDir, "Library/Containers/com.docker.docker/Data/log/host/inference-llama.cpp-server.log")
			case "windows", "linux":
				baseDir := homeDir
				if runtime.GOOS == "linux" {
					if !isWSL() {
						return fmt.Errorf("log viewing on native Linux is only supported in standalone mode")
					}
					// When running inside WSL2 with Docker Desktop, the log files
					// are on the Windows host filesystem mounted under /mnt/.
					winHomeDir, wslErr := windowsHomeDirFromWSL(cmd.Context())
					if wslErr != nil {
						return fmt.Errorf("unable to determine Windows home directory from WSL2: %w", wslErr)
					}
					baseDir = winHomeDir
				}
				serviceLogPath = filepath.Join(baseDir, "AppData/Local/Docker/log/host/inference.log")
				runtimeLogPath = filepath.Join(baseDir, "AppData/Local/Docker/log/host/inference-llama.cpp-server.log")
			default:
				return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
			}

			if noEngines {
				err = printMergedLog(cmd.OutOrStdout(), serviceLogPath, "")
				if err != nil {
					return err
				}
			} else {
				err = printMergedLog(cmd.OutOrStdout(), serviceLogPath, runtimeLogPath)
				if err != nil {
					return err
				}
			}

			if !follow {
				return nil
			}

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
			defer cancel()

			g, ctx := errgroup.WithContext(ctx)

			// Poll mode is needed when tailing files over a mounted filesystem
			// (Windows or WSL2 accessing the Windows host via /mnt/).
			pollMode := runtime.GOOS == "windows" || (runtime.GOOS == "linux" && isWSL())

			g.Go(func() error {
				t, err := tail.TailFile(
					serviceLogPath, tail.Config{Location: &tail.SeekInfo{Offset: 0, Whence: io.SeekEnd}, Follow: true, ReOpen: true, Poll: pollMode},
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
						runtimeLogPath, tail.Config{Location: &tail.SeekInfo{Offset: 0, Whence: io.SeekEnd}, Follow: true, ReOpen: true, Poll: pollMode},
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
		},
		ValidArgsFunction: completion.NoComplete,
	}
	c.Flags().BoolVarP(&follow, "follow", "f", false, "View logs with real-time streaming")
	c.Flags().BoolVar(&noEngines, "no-engines", false, "Exclude inference engine logs from the output")
	return c
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
