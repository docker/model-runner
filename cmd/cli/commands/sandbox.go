package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var allowedSandboxTools = map[string]struct{}{
	"sbx": {},
}

func newSandboxConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config <key> <value>",
		Short: "Set model runner configuration values",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			key := args[0]
			value := args[1]

			if key != "sandbox.tool" {
				return fmt.Errorf("unsupported config key %q", key)
			}

			if err := validateSandboxTool(value); err != nil {
				return err
			}

			return writeSandboxToolConfig(value)
		},
	}
}

func validateSandboxTool(tool string) error {
	if _, ok := allowedSandboxTools[tool]; !ok {
		return fmt.Errorf("unsupported sandbox tool %q", tool)
	}

	return nil
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
