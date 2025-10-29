package commands

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/docker/model-runner/cmd/cli/pkg/types"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/cmd/cli/pkg/standalone"
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
				log, err := dockerClient.ContainerLogs(cmd.Context(), ctrID, container.LogsOptions{
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
			switch {
			case runtime.GOOS == "darwin":
				serviceLogPath = filepath.Join(homeDir, "Library/Containers/com.docker.docker/Data/log/host/inference.log")
				runtimeLogPath = filepath.Join(homeDir, "Library/Containers/com.docker.docker/Data/log/host/inference-llama.cpp-server.log")
			case runtime.GOOS == "windows":
				serviceLogPath = filepath.Join(homeDir, "AppData/Local/Docker/log/host/inference.log")
				runtimeLogPath = filepath.Join(homeDir, "AppData/Local/Docker/log/host/inference-llama.cpp-server.log")
			default:
				return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
			}

			if noEngines {
				err = printMergedLog(serviceLogPath, "")
				if err != nil {
					return err
				}
			} else {
				err = printMergedLog(serviceLogPath, runtimeLogPath)
				if err != nil {
					return err
				}
			}

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
			defer cancel()

			g, ctx := errgroup.WithContext(ctx)

			g.Go(func() error {
				t, err := tail.TailFile(
					serviceLogPath, tail.Config{Location: &tail.SeekInfo{Offset: 0, Whence: io.SeekEnd}, Follow: follow, ReOpen: follow},
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
						fmt.Println(line.Text)
					case <-ctx.Done():
						return t.Stop()
					}
				}
			})

			if follow && !noEngines {
				// Show inference engines logs if `follow` is enabled
				// and the engines logs have not been skipped by setting `--no-engines`.
				g.Go(func() error {
					t, err := tail.TailFile(
						runtimeLogPath, tail.Config{Location: &tail.SeekInfo{Offset: 0, Whence: io.SeekEnd}, Follow: follow, ReOpen: follow},
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
							fmt.Println(line.Text)
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

var timestampRe = regexp.MustCompile(`\[(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z)\].*`)

const timeFmt = "2006-01-02T15:04:05.000000000Z"

func printTillFirstTimestamp(logScanner *bufio.Scanner) (time.Time, string) {
	if logScanner == nil {
		return time.Time{}, ""
	}

	for logScanner.Scan() {
		text := logScanner.Text()
		match := timestampRe.FindStringSubmatch(text)
		if len(match) == 2 {
			timestamp, err := time.Parse(timeFmt, match[1])
			if err != nil {
				println(text)
				continue
			}
			return timestamp, text
		} else {
			println(text)
		}
	}
	return time.Time{}, ""
}

func printMergedLog(logPath1, logPath2 string) error {
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
	var log1Line string
	var log2Name string

	timestamp1, log1Line = printTillFirstTimestamp(logScanner1)
	timestamp2, log2Name = printTillFirstTimestamp(logScanner2)

	for log1Line != "" && log2Name != "" {
		for log1Line != "" && timestamp1.Before(timestamp2) {
			println(log1Line)
			timestamp1, log1Line = printTillFirstTimestamp(logScanner1)
		}
		for log2Name != "" && timestamp2.Before(timestamp1) {
			println(log2Name)
			timestamp2, log2Name = printTillFirstTimestamp(logScanner2)
		}
	}

	if log1Line != "" {
		for logScanner1.Scan() {
			println(logScanner1.Text())
		}
	}
	if log2Name != "" {
		for logScanner2.Scan() {
			println(logScanner2.Text())
		}
	}

	return nil
}
