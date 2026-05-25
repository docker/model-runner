package commands

import (
	"fmt"

	"github.com/docker/cli/cli-plugins/plugin"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/spf13/cobra"
)

// dockerCLI is the Docker CLI environment associated with the command.
var dockerCLI *command.DockerCli

// globalOptions holds the Docker client options used in standalone mode. It is
// set during NewRootCmd and referenced by initDockerCLI.
var globalOptions *flags.ClientOptions

// initDockerCLI performs Docker CLI / plugin initialisation. It is called by
// both the root PersistentPreRunE and the context-command PersistentPreRunE.
// After this call dockerCLI is set and the Docker config is available.
func initDockerCLI(cmd *cobra.Command, args []string, cli *command.DockerCli, opts *flags.ClientOptions) error {
	if plugin.RunningStandalone() {
		opts.SetDefaultOptions(cmd.Root().Flags())
		if err := cli.Initialize(opts); err != nil {
			return fmt.Errorf("unable to configure CLI: %w", err)
		}
	} else if err := plugin.PersistentPreRunE(cmd, args); err != nil {
		return err
	}
	dockerCLI = cli
	return nil
}

// initModelRunner detects the active Model Runner backend and initialises the
// shared desktopClient. It must be called after initDockerCLI.
func initModelRunner(cmd *cobra.Command, cli *command.DockerCli) error {
	var err error
	modelRunner, err = desktop.DetectContext(cmd.Context(), cli, asPrinter(cmd))
	if err != nil {
		return fmt.Errorf("unable to detect model runner context: %w", err)
	}
	desktopClient = desktop.New(modelRunner)
	return nil
}

// getDockerCLI is an accessor for dockerCLI that can be passed to other
// packages.
func getDockerCLI() *command.DockerCli {
	return dockerCLI
}

// modelRunner is the model runner context. It is initialized by the root
// command's PersistentPreRunE.
var modelRunner *desktop.ModelRunnerContext

// desktopClient is the model runner client. It is initialized by the root
// command's PersistentPreRunE.
var desktopClient *desktop.Client

// getDesktopClient is an accessor for desktopClient that can be passed to other
// packages.
func getDesktopClient() *desktop.Client {
	return desktopClient
}

func NewRootCmd(cli *command.DockerCli) *cobra.Command {
	// Set up the root command.
	rootCmd := &cobra.Command{
		Use:   "model",
		Short: "Docker Model Runner",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := initDockerCLI(cmd, args, cli, globalOptions); err != nil {
				return err
			}
			return initModelRunner(cmd, cli)
		},
		// If running standalone, then we'll register global Docker flags as
		// top-level flags on the root command, so we'll have to set
		// TraverseChildren in order for those flags to be inherited. We could
		// instead register them as PersistentFlags, but our approach here
		// better matches the behavior of the Docker CLI, where these flags
		// affect all commands, but don't show up in the help output of all
		// commands a "Global Flags".
		TraverseChildren: plugin.RunningStandalone(),
	}

	// Initialize client options and register their flags if running in
	// standalone mode.
	if plugin.RunningStandalone() {
		globalOptions = flags.NewClientOptions()
		globalOptions.InstallFlags(rootCmd.Flags())
	}

	// Runner management commands - these manage the runner itself and don't need automatic runner initialization.
	rootCmd.AddCommand(
		newVersionCmd(),
		newContextCmd(cli),
		newInstallRunner(),
		newUninstallRunner(),
		newStartRunner(),
		newStopRunner(),
		newRestartRunner(),
		newReinstallRunner(),
		newSearchCmd(),
		newSkillsCmd(),
	)
	rootCmd.AddCommand(newGatewayCmd())

	// Commands that require a running model runner. These are wrapped to ensure the standalone runner is available.
	for _, cmd := range []*cobra.Command{
		newStatusCmd(),
		newPullCmd(),
		newPushCmd(),
		newListCmd(),
		newLogsCmd(),
		newRemoveCmd(),
		newInspectCmd(),
		newShowCmd(),
		newComposeCmd(),
		newLaunchCmd(),
		newTagCmd(),
		newConfigureCmd(),
		newPSCmd(),
		newDFCmd(),
		newUnloadCmd(),
		newRequestsCmd(),
		newPurgeCmd(),
		newBenchCmd(),
	} {
		rootCmd.AddCommand(withStandaloneRunner(cmd))
	}

	// run command handles standalone runner initialization itself (needs debug flag)
	rootCmd.AddCommand(newRunCmd())

	// package command handles standalone runner initialization itself (only when not pushing)
	rootCmd.AddCommand(newPackagedCmd())

	return rootCmd
}
