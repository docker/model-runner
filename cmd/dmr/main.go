// Command dmr is the unified Docker Model Runner daemon and client.
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/docker/cli/cli/command"
	"github.com/docker/model-runner/cmd/cli/commands"
	"github.com/docker/model-runner/pkg/server"
	"github.com/spf13/cobra"
)

var Version = "dev"

const defaultHost = "http://localhost:12434"
const defaultPort = "12434"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "config":
			return runConfig(os.Args[2:])
		case "launch":
			return runLaunch(os.Args[2:])
		}
	}

	cli, err := command.NewDockerCli()
	if err != nil {
		return fmt.Errorf("unable to initialize CLI: %w", err)
	}

	root := commands.NewRootCmd(cli)
	root.Use = "dmr"
	root.Short = "Docker Model Runner"

	if os.Getenv("MODEL_RUNNER_HOST") == "" {
		if err := os.Setenv("MODEL_RUNNER_HOST", defaultHost); err != nil {
			return fmt.Errorf("unable to set MODEL_RUNNER_HOST: %w", err)
		}
	}

	root.AddCommand(newServeCmd())

	return root.Execute()
}

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the Docker Model Runner daemon",
		// skip Docker CLI init; serve runs the daemon directly
		PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
		RunE: func(cmd *cobra.Command, _ []string) error {
			if os.Getenv("MODEL_RUNNER_PORT") == "" {
				if err := os.Setenv("MODEL_RUNNER_PORT", defaultPort); err != nil {
					return fmt.Errorf("unable to set MODEL_RUNNER_PORT: %w", err)
				}
			}

			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			return server.Run(ctx, server.Config{Version: Version})
		},
	}
}

func runConfig(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: dmr config sandbox.tool <tool>")
	}

	key := args[0]
	value := args[1]

	if key != "sandbox.tool" {
		return fmt.Errorf("unsupported config key %q", key)
	}

	return writeSandboxToolConfig(value)
}

func runLaunch(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: dmr launch <tool> [args...]")
	}

	sandboxTool, err := readSandboxToolConfig()
	if err != nil {
		return err
	}

	if sandboxTool == "" {
		return fmt.Errorf("sandbox.tool is not configured. Run: dmr config sandbox.tool <tool>")
	}

	cmd := exec.Command(sandboxTool, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func dmrConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("unable to determine config directory: %w", err)
	}

	return filepath.Join(configDir, "dmr", "config.toml"), nil
}

func writeSandboxToolConfig(tool string) error {
	path, err := dmrConfigPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("unable to create config directory: %w", err)
	}

	content := fmt.Sprintf("[sandbox]\ntool = %q\n", tool)

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("unable to write config: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("unable to secure config permissions: %w", err)
	}

	return nil
}

func readSandboxToolConfig() (string, error) {
	path, err := dmrConfigPath()
	if err != nil {
		return "", err
	}

	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("unable to read config: %w", err)
	}
	defer file.Close()

	inSandboxSection := false
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inSandboxSection = line == "[sandbox]"
			continue
		}

		if !inSandboxSection {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		if strings.TrimSpace(key) != "tool" {
			continue
		}

		return strings.Trim(strings.TrimSpace(value), `"`), nil
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("unable to parse config: %w", err)
	}

	return "", nil
}
