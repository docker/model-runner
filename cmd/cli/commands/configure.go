package commands

import (
	"fmt"

	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/pkg/inference/scheduling"
	"github.com/spf13/cobra"
)

func newConfigureCmd() *cobra.Command {
	var opts scheduling.ConfigureRequest

	c := &cobra.Command{
		Use:    "configure [--context-size=<n>] MODEL [-- <runtime-flags...>]",
		Short:  "Configure runtime options for a model",
		Hidden: true,
		Args: func(cmd *cobra.Command, args []string) error {
			argsBeforeDash := cmd.ArgsLenAtDash()
			if argsBeforeDash == -1 {
				// No "--" used, so we need exactly 1 total argument.
				if len(args) != 1 {
					return fmt.Errorf(
						"Exactly one model must be specified, got %d: %v\n\n"+
							"See 'docker model configure --help' for more information",
						len(args), args)
				}
			} else {
				// Has "--", so we need exactly 1 argument before it.
				if argsBeforeDash != 1 {
					return fmt.Errorf(
						"Exactly one model must be specified before --, got %d\n\n"+
							"See 'docker model configure --help' for more information",
						argsBeforeDash)
				}
			}
			opts.Model = args[0]
			opts.RuntimeFlags = args[1:]
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return desktopClient.ConfigureBackend(opts)
		},
		ValidArgsFunction: completion.ModelNames(getDesktopClient, -1),
	}

	c.Flags().Int64Var(&opts.ContextSize, "context-size", -1, "context size (in tokens)")
	return c
}
