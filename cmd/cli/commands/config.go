package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/docker/model-runner/cmd/cli/iniconfig"
	"github.com/spf13/cobra"
)

// defaultConfigPath returns the default (global/user-level) config file path.
// It honours XDG_CONFIG_HOME when set:
//
//	$XDG_CONFIG_HOME/model-runner/config
//	~/.config/model-runner/config  (fallback)
func defaultConfigPath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "model-runner", "config"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot find the config directory (set XDG_CONFIG_HOME or use --file): %w", err)
	}
	return filepath.Join(home, ".config", "model-runner", "config"), nil
}

// systemConfigPath returns the system-wide config file path.
func systemConfigPath() string {
	if runtime.GOOS == "windows" {
		if pd := os.Getenv("ProgramData"); pd != "" {
			return filepath.Join(pd, "model-runner", "config")
		}
		return `C:\ProgramData\model-runner\config`
	}
	return "/etc/model-runner/config"
}

// resolveConfigPath picks the config file to operate on, given the flags.
// Exactly one of global, system, or file may be set.
func resolveConfigPath(global, system bool, file string) (string, error) {
	count := 0
	for _, set := range []bool{global, system, file != ""} {
		if set {
			count++
		}
	}
	if count > 1 {
		return "", fmt.Errorf("only one of --global, --system, or --file may be specified")
	}
	switch {
	case system:
		return systemConfigPath(), nil
	case file != "":
		return file, nil
	default:
		// --global is the default
		return defaultConfigPath()
	}
}

// locationFlags holds the --global/--system/--file flags shared by subcommands.
type locationFlags struct {
	global, system bool
	file           string
}

// add registers the flags on cmd.
func (l *locationFlags) add(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&l.global, "global", false, "use the global (user-level) config file")
	cmd.Flags().BoolVar(&l.system, "system", false, "use the system-wide config file")
	cmd.Flags().StringVarP(&l.file, "file", "f", "", "use a specific config file")
}

// path resolves the config file selected by the flags.
func (l *locationFlags) path() (string, error) {
	return resolveConfigPath(l.global, l.system, l.file)
}

// load resolves the config path and loads it.
func (l *locationFlags) load() (*iniconfig.File, error) {
	path, err := l.path()
	if err != nil {
		return nil, err
	}
	return iniconfig.Load(path)
}

// printLine writes s to stdout, prefixed with the file path if showOrigin.
func printLine(cmd *cobra.Command, f *iniconfig.File, showOrigin bool, s string) {
	if showOrigin {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "file:%s\t", f.Path())
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), s)
}

// newConfigCmd returns the top-level "config" command.
func newConfigCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "config",
		Short: "Read and write model-runner config file values",
		Long: `Read and write model-runner config file values.

The config file uses an INI format with sections and key=value pairs:

    [section]
        key = value
    [section "subsection"]
        key = value

Keys are specified in dot notation: section.key or section.subsection.key.

The default file is $XDG_CONFIG_HOME/model-runner/config, falling back to
~/.config/model-runner/config when XDG_CONFIG_HOME is not set.

Examples:
    model-cli config set user.name "Alice"
    model-cli config get user.name
    model-cli config list
    model-cli config unset user.name
    model-cli config edit`,
		// Do not run a PersistentPreRunE that requires a running model-runner;
		// config is pure local-file work.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	c.AddCommand(
		newConfigGetCmd(),
		newConfigSetCmd(),
		newConfigUnsetCmd(),
		newConfigListCmd(),
		newConfigEditCmd(),
	)
	return c
}

// newConfigGetCmd implements "model-cli config get <key>".
func newConfigGetCmd() *cobra.Command {
	var (
		loc        locationFlags
		defaultVal string
		hasDefault bool
		showAll    bool
		showOrigin bool
	)

	c := &cobra.Command{
		Use:   "get <key>",
		Short: "Get the value of a config key",
		Long: `Get the value of a config key.

Prints the value of the given key to stdout. If the key appears multiple times
(multi-valued), the last value is printed. Use --all to print all values.

Exit status is 1 if the key is not found (unless --default is given).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get/GetAll treat a malformed key as missing; reject it up front.
			key := args[0]
			if _, _, _, err := iniconfig.ParseKey(key); err != nil {
				return err
			}
			f, err := loc.load()
			if err != nil {
				return err
			}

			var vals []string
			if showAll {
				vals = f.GetAll(key)
			} else if v, ok := f.Get(key); ok {
				vals = []string{v}
			}

			if len(vals) == 0 {
				if hasDefault {
					printLine(cmd, f, false, defaultVal)
					return nil
				}
				return fmt.Errorf("key not found: %s", key)
			}
			for _, v := range vals {
				printLine(cmd, f, showOrigin, v)
			}
			return nil
		},
	}

	loc.add(c)
	c.Flags().StringVar(&defaultVal, "default", "", "value to emit if the key is not set")
	c.Flags().BoolVar(&showAll, "all", false, "print all values for multi-valued keys")
	c.Flags().BoolVar(&showOrigin, "show-origin", false, "show the origin (file path) of each value")
	// Track whether --default was explicitly provided.
	c.PreRunE = func(cmd *cobra.Command, args []string) error {
		hasDefault = cmd.Flags().Changed("default")
		return nil
	}

	return c
}

// newConfigSetCmd implements "model-cli config set <key> <value>".
func newConfigSetCmd() *cobra.Command {
	var loc locationFlags

	c := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a config key to a value",
		Long: `Set a config key to a value.

If the key already exists its value is replaced. The file is written atomically.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := loc.load()
			if err != nil {
				return err
			}
			return f.Set(args[0], args[1])
		},
	}

	loc.add(c)
	return c
}

// newConfigUnsetCmd implements "model-cli config unset <key>".
func newConfigUnsetCmd() *cobra.Command {
	var loc locationFlags

	c := &cobra.Command{
		Use:   "unset <key>",
		Short: "Remove a config key",
		Long:  `Remove a config key (and all its values) from the file.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := loc.load()
			if err != nil {
				return err
			}
			return f.Unset(args[0])
		},
	}

	loc.add(c)
	return c
}

// newConfigListCmd implements "model-cli config list".
func newConfigListCmd() *cobra.Command {
	var loc locationFlags
	var showOrigin bool

	c := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all config key/value pairs",
		Long:    `List all key=value pairs from the config file, one per line.`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := loc.load()
			if err != nil {
				return err
			}
			for _, e := range f.Entries() {
				printLine(cmd, f, showOrigin, e.Format())
			}
			return nil
		},
	}

	loc.add(c)
	c.Flags().BoolVar(&showOrigin, "show-origin", false, "show the origin (file path) of each value")
	return c
}

// editorCommand returns the editor command and its args from VISUAL, then
// EDITOR, skipping empty values, then the platform default.
func editorCommand() []string {
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if parts := strings.Fields(os.Getenv(env)); len(parts) > 0 {
			return parts
		}
	}
	if runtime.GOOS == "windows" {
		return []string{"notepad"}
	}
	return []string{"vi"}
}

// newConfigEditCmd implements "model-cli config edit".
func newConfigEditCmd() *cobra.Command {
	var loc locationFlags

	c := &cobra.Command{
		Use:   "edit",
		Short: "Open the config file in your editor",
		Long: `Open the config file in the default editor.

The editor is determined by the VISUAL or EDITOR environment variables,
falling back to vi on Unix and notepad on Windows.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := loc.path()
			if err != nil {
				return err
			}
			// Ensure the file (and its parent directory) exist so the editor
			// has something to open.
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if _, err := os.Stat(path); os.IsNotExist(err) {
				// Create with 0600, config files may hold sensitive values.
				f, err2 := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
				if err2 != nil {
					return err2
				}
				_ = f.Close()
			}

			// VISUAL/EDITOR may contain arguments (e.g. "code --wait").
			parts := editorCommand()
			editorArgs := append(parts[1:], path)
			//nolint:gosec // editor is a user-controlled input, which is intentional
			editorCmd := exec.CommandContext(cmd.Context(), parts[0], editorArgs...)
			editorCmd.Stdin = os.Stdin
			editorCmd.Stdout = os.Stdout
			editorCmd.Stderr = os.Stderr
			return editorCmd.Run()
		},
	}

	loc.add(c)
	return c
}
