package commands

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/docker/cli/cli/command"
	"github.com/docker/model-runner/cmd/cli/commands/formatter"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/cmd/cli/pkg/modelctx"
	"github.com/docker/model-runner/cmd/cli/pkg/standalone"
	"github.com/docker/model-runner/cmd/cli/pkg/types"
	"github.com/spf13/cobra"
)

// defaultContextDetectTimeout is the maximum time allowed for detecting the
// engine kind when building the synthetic "default" context row.
const defaultContextDetectTimeout = 2 * time.Second

// newContextCmd returns the "docker model context" parent command. Its
// subcommands manage named Model Runner contexts stored on disk, so they do
// not require a running backend and override PersistentPreRunE accordingly.
func newContextCmd(cli *command.DockerCli) *cobra.Command {
	c := &cobra.Command{
		Use:   "context",
		Short: "Manage Docker Model Runner contexts",
		// Context management commands need only CLI initialisation, not a
		// running backend. Override PersistentPreRunE to skip DetectContext.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initDockerCLI(cmd, args, cli, globalOptions)
		},
	}

	c.AddCommand(
		newContextCreateCmd(),
		newContextUseCmd(),
		newContextLsCmd(),
		newContextRmCmd(),
		newContextInspectCmd(),
	)
	return c
}

// contextStore opens the context store using the Docker config directory
// derived from the current CLI configuration.
func contextStore() (*modelctx.Store, error) {
	dir, err := dockerConfigDir()
	if err != nil {
		return nil, fmt.Errorf("unable to determine Docker config directory: %w", err)
	}
	return modelctx.New(dir)
}

// dockerConfigDir returns the Docker configuration directory. It honours the
// DOCKER_CONFIG environment variable and falls back to ~/.docker.
func dockerConfigDir() (string, error) {
	if dockerCLI != nil && dockerCLI.ConfigFile() != nil {
		return filepath.Dir(dockerCLI.ConfigFile().Filename), nil
	}
	// Fallback used during testing or when CLI is not yet initialised.
	if d := os.Getenv("DOCKER_CONFIG"); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("unable to determine home directory: %w", err)
	}
	return filepath.Join(home, ".docker"), nil
}

// resolveDefaultContext attempts to detect the Docker engine kind for the
// synthetic "default" context row. When the CLI is available it probes the
// Docker daemon (with a short timeout) and returns a descriptive host and
// description derived from the detected engine kind. If detection fails or
// the CLI is unavailable it falls back to generic strings.
func resolveDefaultContext(ctx context.Context) (host, description string) {
	const fallbackHost = "(auto-detect)"
	const fallbackDescription = "Auto-detected Docker context"

	if dockerCLI == nil {
		return fallbackHost, fallbackDescription
	}

	detectCtx, cancel := context.WithTimeout(ctx, defaultContextDetectTimeout)
	defer cancel()

	kind := desktop.DetectEngineKind(detectCtx, dockerCLI)
	description = "Model Runner on " + kind.String()

	// Check whether TLS is enabled so the displayed scheme and port match
	// what DetectContext would actually use at runtime.
	useTLS := os.Getenv("MODEL_RUNNER_TLS") == "true"

	switch kind {
	case types.ModelRunnerEngineKindDesktop:
		host = kind.String()
	case types.ModelRunnerEngineKindCloud:
		if useTLS {
			host = "https://localhost:" + strconv.Itoa(standalone.DefaultTLSPortCloud)
		} else {
			host = "http://localhost:" + strconv.Itoa(standalone.DefaultControllerPortCloud)
		}
	case types.ModelRunnerEngineKindMoby, types.ModelRunnerEngineKindMobyManual:
		if useTLS {
			host = "https://localhost:" + strconv.Itoa(standalone.DefaultTLSPortMoby)
		} else {
			host = "http://localhost:" + strconv.Itoa(standalone.DefaultControllerPortMoby)
		}
	}

	return host, description
}

// newContextCreateCmd returns the "context create" command.
func newContextCreateCmd() *cobra.Command {
	var (
		host          string
		tls           bool
		tlsSkipVerify bool
		tlsCACert     string
		description   string
	)

	c := &cobra.Command{
		Use:   "create NAME",
		Short: "Create a named Model Runner context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			// Validate and normalise the host URL.
			if host == "" {
				return fmt.Errorf("--host is required")
			}

			u, err := url.ParseRequestURI(host)
			if err != nil {
				return fmt.Errorf("invalid --host URL: %w", err)
			}
			if u.Scheme == "" || u.Host == "" {
				return fmt.Errorf("invalid --host URL: must include scheme and host, e.g. http://192.168.1.100:12434")
			}
			if u.Scheme != "http" && u.Scheme != "https" {
				return fmt.Errorf("invalid --host URL: unsupported scheme %q (must be http or https)", u.Scheme)
			}

			// Normalise the host string.
			host = u.String()

			// Validate the CA cert path if provided.
			tlsCACertAbs := ""
			if tlsCACert != "" {
				abs, err := filepath.Abs(tlsCACert)
				if err != nil {
					return fmt.Errorf("invalid --tls-ca-cert path: %w", err)
				}
				if _, err := os.ReadFile(abs); err != nil {
					return fmt.Errorf(
						"--tls-ca-cert: cannot read %q: %w", abs, err,
					)
				}
				tlsCACertAbs = abs
			}

			store, err := contextStore()
			if err != nil {
				return fmt.Errorf("unable to open context store: %w", err)
			}

			cfg := modelctx.ContextConfig{
				Host: host,
				TLS: modelctx.TLSConfig{
					Enabled:    tls,
					SkipVerify: tlsSkipVerify,
					CACert:     tlsCACertAbs,
				},
				Description: description,
				CreatedAt:   time.Now().UTC(),
			}
			if err := store.Create(name, cfg); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Context %q created.\n", name)
			return nil
		},
	}

	c.Flags().StringVar(&host, "host", "",
		"Model Runner API base URL (e.g. http://192.168.1.100:12434)")
	c.Flags().BoolVar(&tls, "tls", false,
		"Enable TLS for connections to this context")
	c.Flags().BoolVar(&tlsSkipVerify, "tls-skip-verify", false,
		"Skip TLS server certificate verification")
	c.Flags().StringVar(&tlsCACert, "tls-ca-cert", "",
		"Path to a custom CA certificate PEM file for TLS verification")
	c.Flags().StringVar(&description, "description", "",
		"Optional human-readable description for this context")
	return c
}

// newContextUseCmd returns the "context use" command.
func newContextUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use NAME",
		Short: "Set the active Model Runner context",
		Long: `Set the active Model Runner context. Pass "default" to revert to
automatic backend detection.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			store, err := contextStore()
			if err != nil {
				return fmt.Errorf("unable to open context store: %w", err)
			}

			if err := store.SetActive(name); err != nil {
				return err
			}

			fmt.Fprintf(
				cmd.OutOrStdout(),
				"Current context is now %q.\n", name,
			)
			return nil
		},
	}
}

// contextListRow holds the data for one row in the "context ls" table.
type contextListRow struct {
	name        string
	host        string
	description string
	active      bool
}

// newContextLsCmd returns the "context ls" command.
func newContextLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List Model Runner contexts",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := contextStore()
			if err != nil {
				return fmt.Errorf("unable to open context store: %w", err)
			}

			contexts, err := store.List()
			if err != nil {
				return fmt.Errorf("unable to list contexts: %w", err)
			}

			activeName, err := store.Active()
			if err != nil {
				return fmt.Errorf("unable to determine active context: %w", err)
			}

			// Warn if MODEL_RUNNER_HOST overrides the active context.
			if envHost := os.Getenv("MODEL_RUNNER_HOST"); envHost != "" {
				fmt.Fprintf(
					cmd.ErrOrStderr(),
					"Warning: MODEL_RUNNER_HOST=%q overrides the active context.\n",
					envHost,
				)
			}

			// Resolve the host and description for the synthetic "default"
			// row by detecting the engine kind (Desktop, Moby, Cloud).
			defaultHost, defaultDescription := resolveDefaultContext(cmd.Context())

			// Build rows: synthetic "default" first, then named contexts sorted.
			rows := []contextListRow{
				{
					name:        modelctx.DefaultContextName,
					host:        defaultHost,
					description: defaultDescription,
					active:      activeName == modelctx.DefaultContextName,
				},
			}

			names := make([]string, 0, len(contexts))
			for n := range contexts {
				names = append(names, n)
			}
			sort.Strings(names)

			for _, n := range names {
				cfg := contexts[n]
				rows = append(rows, contextListRow{
					name:        n,
					host:        cfg.Host,
					description: cfg.Description,
					active:      activeName == n,
				})
			}

			var buf bytes.Buffer
			table := newTable(&buf)
			table.Header([]string{"NAME", "HOST", "DESCRIPTION", "CURRENT"})
			for _, row := range rows {
				current := ""
				if row.active {
					current = "*"
				}
				table.Append([]string{
					row.name,
					row.host,
					row.description,
					current,
				})
			}
			table.Render()

			fmt.Fprint(cmd.OutOrStdout(), buf.String())
			return nil
		},
	}
}

// newContextRmCmd returns the "context rm" command.
func newContextRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "rm NAME [NAME...]",
		Aliases: []string{"remove"},
		Short:   "Remove one or more Model Runner contexts",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := contextStore()
			if err != nil {
				return fmt.Errorf("unable to open context store: %w", err)
			}

			// Attempt removal of all named contexts; collect errors.
			var errs []error
			for _, name := range args {
				if err := store.Remove(name); err != nil {
					errs = append(errs, fmt.Errorf("%s: %w", name, err))
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Context %q removed.\n", name)
			}

			if len(errs) > 0 {
				for _, e := range errs {
					fmt.Fprintln(cmd.ErrOrStderr(), "Error:", e)
				}
				return fmt.Errorf("one or more contexts could not be removed")
			}
			return nil
		},
	}
}

// namedContextInspect is the JSON-serialisable representation of a named
// context returned by "context inspect".
type namedContextInspect struct {
	Name string `json:"name"`
	modelctx.ContextConfig
}

// newContextInspectCmd returns the "context inspect" command.
func newContextInspectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect NAME [NAME...]",
		Short: "Display detailed information about one or more contexts",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := contextStore()
			if err != nil {
				return fmt.Errorf("unable to open context store: %w", err)
			}

			// Resolve the default context info once (lazily) so that
			// repeated "default" args do not trigger multiple probes.
			var defaultHost, defaultDescription string
			defaultResolved := false

			results := make([]namedContextInspect, 0, len(args))
			for _, name := range args {
				if name == modelctx.DefaultContextName {
					if !defaultResolved {
						defaultHost, defaultDescription = resolveDefaultContext(cmd.Context())
						defaultResolved = true
					}
					results = append(results, namedContextInspect{
						Name: modelctx.DefaultContextName,
						ContextConfig: modelctx.ContextConfig{
							Host:        defaultHost,
							Description: defaultDescription,
						},
					})
					continue
				}
				cfg, err := store.Get(name)
				if err != nil {
					return err
				}
				results = append(results, namedContextInspect{
					Name:          name,
					ContextConfig: cfg,
				})
			}

			output, err := formatter.ToStandardJSON(results)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), output)
			return nil
		},
	}
}
