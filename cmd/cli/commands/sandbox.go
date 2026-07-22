package commands

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var allowedSandboxTools = map[string]struct{}{
	"sbx": {},
}

func newSandboxConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sandbox.tool <tool>",
		Short: "Set the sandbox tool",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			tool, err := validateSandboxTool(args[0])
			if err != nil {
				return err
			}

			return writeSandboxToolConfig(tool)
		},
	}
}

func validateSandboxTool(tool string) (string, error) {
	if _, ok := allowedSandboxTools[tool]; !ok {
		return "", fmt.Errorf("unsupported sandbox tool %q", tool)
	}

	return tool, nil
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

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("unable to write config: %w", err)
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
func runSandboxTool(cmd *cobra.Command, sandboxTool string, args []string, dryRun bool) error {
	validatedSandboxTool, err := validateSandboxTool(sandboxTool)
	if err != nil {
		return err
	}

	if dryRun {
		cmd.Printf("%s %s\n", validatedSandboxTool, strings.Join(args, " "))
		return nil
	}

	switch validatedSandboxTool {
	case "sbx":
		launchCmd := exec.Command("sbx", args...)
		launchCmd.Stdin = os.Stdin
		launchCmd.Stdout = os.Stdout
		launchCmd.Stderr = os.Stderr

		return launchCmd.Run()
	default:
		return fmt.Errorf("unsupported sandbox tool %q", validatedSandboxTool)
	}
}
func configuredSandboxTool() (string, error) {
	sandboxTool, err := readSandboxToolConfig()
	if err != nil {
		return "", err
	}

	if sandboxTool == "" {
		return "", nil
	}

	return validateSandboxTool(sandboxTool)
}
