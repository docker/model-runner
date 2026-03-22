package commands

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/cmd/cli/pkg/standalone"
	"github.com/docker/model-runner/pkg/distribution/oci/reference"
	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/docker/model-runner/pkg/inference/backends/diffusers"
	"github.com/docker/model-runner/pkg/inference/backends/llamacpp"
	"github.com/docker/model-runner/pkg/inference/backends/vllm"
	dmrm "github.com/docker/model-runner/pkg/inference/models"
	"github.com/moby/term"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/spf13/cobra"
)

const (
	defaultOrg = "ai"
	defaultTag = "latest"
)

const (
	enableViaCLI = "Enable Docker Model Runner via the CLI → docker desktop enable model-runner"
	enableViaGUI = "Enable Docker Model Runner via the GUI → Go to Settings->AI->Enable Docker Model Runner"
	enableVLLM   = "It looks like you're trying to use a model for vLLM → docker model reinstall-runner --backend vllm --gpu cuda"
)

// getDefaultRegistry returns the default registry, checking for environment override
// If DEFAULT_REGISTRY environment variable is set, it returns that value
// Otherwise, it returns reference.DefaultRegistry ("index.docker.io")
func getDefaultRegistry() string {
	if defaultReg := os.Getenv("DEFAULT_REGISTRY"); defaultReg != "" {
		return defaultReg
	}
	return reference.DefaultRegistry
}

var errNotRunning = fmt.Errorf("Docker Model Runner is not running. Please start it and try again.\n")

var errBackendInstallationCancelled = errors.New("backend installation cancelled")

func handleClientError(err error, message string) error {
	if errors.Is(err, desktop.ErrServiceUnavailable) {
		err = errNotRunning
		var buf bytes.Buffer
		printNextSteps(&buf, []string{enableViaCLI, enableViaGUI})
		return fmt.Errorf("%w\n%s", err, strings.TrimRight(buf.String(), "\n"))
	} else if strings.Contains(err.Error(), vllm.ErrorNotFound.Error()) {
		// Handle `run` error.
		var buf bytes.Buffer
		printNextSteps(&buf, []string{enableVLLM})
		return fmt.Errorf("%w\n%s", err, strings.TrimRight(buf.String(), "\n"))
	}
	return fmt.Errorf("%s: %w", message, err)
}

// commandPrinter wraps a cobra.Command to implement standalone.StatusPrinter
type commandPrinter struct {
	cmd *cobra.Command
}

// Printf implements StatusPrinter.Printf by delegating to cobra.Command.Printf
func (cp *commandPrinter) Printf(format string, args ...any) {
	cp.cmd.Printf(format, args...)
}

// Println implements StatusPrinter.Println by delegating to cobra.Command.Println
func (cp *commandPrinter) Println(args ...any) {
	cp.cmd.Println(args...)
}

// PrintErrf implements StatusPrinter.PrintErrf by delegating to cobra.Command.PrintErrf
func (cp *commandPrinter) PrintErrf(format string, args ...any) {
	cp.cmd.PrintErrf(format, args...)
}

// Write implements StatusPrinter.Write by delegating to cobra.Command's output writer
func (cp *commandPrinter) Write(p []byte) (n int, err error) {
	return cp.cmd.OutOrStdout().Write(p)
}

// GetFdInfo returns the file descriptor and terminal status of the command's output
func (cp *commandPrinter) GetFdInfo() (fd uintptr, isTerminal bool) {
	out := cp.cmd.OutOrStdout()

	if file, ok := out.(*os.File); ok {
		return term.GetFdInfo(file)
	}

	// For progress display, we care about whether stdout is a terminal
	// Even if cobra wraps the output, checking os.Stdout directly is appropriate
	// because that's where the visual progress bars should be displayed
	return term.GetFdInfo(os.Stdout)
}

// asPrinter wraps a cobra.Command to implement standalone.StatusPrinter
func asPrinter(cmd *cobra.Command) standalone.StatusPrinter {
	return &commandPrinter{cmd: cmd}
}

// stripDefaultsFromModelName removes the default "ai/" prefix, default registry, and ":latest" tag for display.
// Examples:
//   - "ai/gemma3:latest" -> "gemma3"
//   - "ai/gemma3:v1" -> "gemma3:v1"
//   - "myorg/gemma3:latest" -> "myorg/gemma3"
//   - "gemma3:latest" -> "gemma3"
//   - "index.docker.io/ai/gemma3:latest" -> "gemma3"
//   - "docker.io/ai/gemma3:latest" -> "gemma3"
//   - "docker.io/myorg/gemma3:latest" -> "myorg/gemma3"
//   - "hf.co/bartowski/model:latest" -> "hf.co/bartowski/model"
func stripDefaultsFromModelName(model string) string {
	// Get the current default registry (checking for environment override)
	defaultRegistry := getDefaultRegistry()

	// Handle the common default registries that are aliases for each other
	// Always handle "index.docker.io" and "docker.io" as defaults regardless of DEFAULT_REGISTRY env var
	// since they are equivalent and commonly used interchangeably
	defaultRegistries := []string{"index.docker.io/", "docker.io/"}
	if defaultRegistry != "" &&
		defaultRegistry != "index.docker.io" &&
		defaultRegistry != "docker.io" {

		// Ensure it has a trailing slash for correct prefix trimming
		if !strings.HasSuffix(defaultRegistry, "/") {
			defaultRegistry += "/"
		}
		// Overwrite the list to contain only the custom registry
		defaultRegistries = []string{defaultRegistry}
	}

	// Check for the common default registries first
	for _, reg := range defaultRegistries {
		if strings.HasPrefix(model, reg) {
			// Remove the registry prefix
			model = strings.TrimPrefix(model, reg)
			break
		}
	}

	// If model has default org prefix (without tag, or with :latest tag), strip the org
	// but preserve other tags
	if strings.HasPrefix(model, defaultOrg+"/") {
		model = strings.TrimPrefix(model, defaultOrg+"/")
	}

	// Check if model has :latest but no slash (no org specified) - strip :latest
	if strings.HasSuffix(model, ":"+defaultTag) {
		model = strings.TrimSuffix(model, ":"+defaultTag)
	}

	// For other cases (ai/ with custom tag, custom org with :latest, etc.), keep as-is
	return model
}

// requireExactArgs returns a cobra.PositionalArgs validator that ensures exactly n arguments are provided
func requireExactArgs(n int, cmdName string, usageArgs string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			return fmt.Errorf(
				"'docker model %s' requires %d argument(s).\n\n"+
					"Usage:  docker model %s %s\n\n"+
					"See 'docker model %s --help' for more information",
				cmdName, n, cmdName, usageArgs, cmdName,
			)
		}
		return nil
	}
}

// requireMinArgs returns a cobra.PositionalArgs validator that ensures at least n arguments are provided
func requireMinArgs(n int, cmdName string, usageArgs string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < n {
			return fmt.Errorf(
				"'docker model %s' requires at least %d argument(s).\n\n"+
					"Usage:  docker model %s %s\n\n"+
					"See 'docker model %s --help' for more information",
				cmdName, n, cmdName, usageArgs, cmdName,
			)
		}
		return nil
	}
}

// runnerFlagOptions holds common runner configuration options
type runnerFlagOptions struct {
	Port       *uint16
	Host       *string
	GpuMode    *string
	Backend    *string
	DoNotTrack *bool
	Debug      *bool
	ProxyCert  *string
	TLS        *bool
	TLSPort    *uint16
	TLSCert    *string
	TLSKey     *string
}

// addRunnerFlags adds common runner flags to a command
func addRunnerFlags(cmd *cobra.Command, opts runnerFlagOptions) {
	if opts.Port != nil {
		cmd.Flags().Uint16Var(opts.Port, "port", 0,
			"Docker container port for Docker Model Runner (default: 12434 for Docker Engine, 12435 for Cloud mode)")
	}
	if opts.Host != nil {
		cmd.Flags().StringVar(opts.Host, "host", "127.0.0.1", "Host address to bind Docker Model Runner")
	}
	if opts.GpuMode != nil {
		cmd.Flags().StringVar(opts.GpuMode, "gpu", "auto", "Specify GPU support (none|auto|cuda|rocm|musa|cann)")
	}
	if opts.Backend != nil {
		cmd.Flags().StringVar(opts.Backend, "backend", "", backendUsage)
	}
	if opts.DoNotTrack != nil {
		cmd.Flags().BoolVar(opts.DoNotTrack, "do-not-track", false, "Do not track models usage in Docker Model Runner")
	}
	if opts.Debug != nil {
		cmd.Flags().BoolVar(opts.Debug, "debug", false, "Enable debug logging")
	}
	if opts.ProxyCert != nil {
		cmd.Flags().StringVar(opts.ProxyCert, "proxy-cert", "", "Path to a CA certificate file for proxy SSL inspection")
	}
	if opts.TLS != nil {
		cmd.Flags().BoolVar(opts.TLS, "tls", false, "Enable TLS/HTTPS for Docker Model Runner API")
	}
	if opts.TLSPort != nil {
		cmd.Flags().Uint16Var(opts.TLSPort, "tls-port", 0,
			"TLS port for Docker Model Runner (default: 12444 for Docker Engine, 12445 for Cloud mode)")
	}
	if opts.TLSCert != nil {
		cmd.Flags().StringVar(opts.TLSCert, "tls-cert", "", "Path to TLS certificate file (auto-generated if not provided)")
	}
	if opts.TLSKey != nil {
		cmd.Flags().StringVar(opts.TLSKey, "tls-key", "", "Path to TLS private key file (auto-generated if not provided)")
	}
}

// newTable creates a new table with Docker CLI-style formatting:
// no borders, no column separators, no header line, left-aligned, and 2-space padding.
func newTable(w io.Writer) *tablewriter.Table {
	return tablewriter.NewTable(w,
		tablewriter.WithRenderer(renderer.NewBlueprint(tw.Rendition{
			Borders: tw.BorderNone,
			Settings: tw.Settings{
				Separators: tw.Separators{
					BetweenColumns: tw.Off,
				},
				Lines: tw.Lines{
					ShowHeaderLine: tw.Off,
				},
			},
		})),
		tablewriter.WithConfig(tablewriter.Config{
			Header: tw.CellConfig{
				Formatting: tw.CellFormatting{
					AutoFormat: tw.Off,
				},
				Alignment: tw.CellAlignment{Global: tw.AlignLeft},
				Padding:   tw.CellPadding{Global: tw.Padding{Left: "", Right: "  "}},
			},
			Row: tw.CellConfig{
				Alignment: tw.CellAlignment{Global: tw.AlignLeft},
				Padding:   tw.CellPadding{Global: tw.Padding{Left: "", Right: "  "}},
			},
		}),
	)
}

func CheckBackendInstalled(backend string) (bool, error) {
	status := desktopClient.Status()
	if status.Error != nil {
		return false, fmt.Errorf("failed to get backend status: %w", status.Error)
	}

	var backendStatus map[string]string
	if err := json.Unmarshal(status.Status, &backendStatus); err != nil {
		return false, fmt.Errorf("failed to parse backend status: %w", err)
	}

	backendState, exists := backendStatus[backend]
	if !exists {
		return false, nil
	}

	state := strings.TrimSpace(strings.ToLower(backendState))
	if strings.HasPrefix(state, "not ") || strings.HasPrefix(state, "error") {
		return false, nil
	}

	return strings.HasPrefix(state, "installed") || strings.HasPrefix(state, "running"), nil
}

func PromptInstallBackend(backend string, cmd *cobra.Command) (bool, error) {
	fmt.Fprintf(cmd.OutOrStdout(), "Backend %q is not installed. Download and install it now? [Y/n]: ", backend)

	reader := bufio.NewReader(cmd.InOrStdin())
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(strings.ToLower(input))
	return input == "" || input == "y" || input == "yes", nil
}

func InstallBackend(backend string) error {
	if err := desktopClient.InstallBackend(backend); err != nil {
		return fmt.Errorf("failed to install backend %s: %w", backend, err)
	}

	return nil
}

func EnsureBackendAvailable(backend string, cmd *cobra.Command) error {
	installed, err := CheckBackendInstalled(backend)
	if err != nil {
		return err
	}

	if installed {
		return nil
	}

	confirm, err := PromptInstallBackend(backend, cmd)
	if err != nil {
		return err
	}

	if !confirm {
		cmd.Printf("Run 'docker model install-runner --backend %s' to install it manually.\n", backend)
		return errBackendInstallationCancelled
	}

	if err := InstallBackend(backend); err != nil {
		return err
	}

	installed, err = CheckBackendInstalled(backend)
	if err != nil {
		return err
	}
	if !installed {
		return fmt.Errorf("backend %q is still not installed; run 'docker model install-runner --backend %s'", backend, backend)
	}

	cmd.Printf("Backend %q installed successfully.\n", backend)
	return nil
}

func GetRequiredBackendFromModelInfo(modelInfo *dmrm.Model) (string, error) {
	config, ok := modelInfo.Config.(*types.Config)
	if !ok {
		return llamacpp.Name, nil
	}

	switch config.Format {
	case types.FormatSafetensors:
		return vllm.Name, nil
	case types.FormatGGUF:
		return llamacpp.Name, nil
	case types.FormatDiffusers:
		return diffusers.Name, nil
	default:
		return llamacpp.Name, nil
	}
}

func printNextSteps(out io.Writer, messages []string) {
	if len(messages) == 0 {
		return
	}
	_, _ = fmt.Fprintln(out, bold("\nWhat's next:"))
	for _, n := range messages {
		_, _ = fmt.Fprintln(out, "   ", n)
	}
}

func bold(s string) string {
	return "\033[1m" + s + "\033[0m"
}
