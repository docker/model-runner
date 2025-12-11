package commands

import (
	"fmt"

	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/spf13/cobra"
)

func newConfigureCmd() *cobra.Command {
	var flags ConfigureFlags

	c := &cobra.Command{
		Use:    "configure [--context-size=<n>] [--speculative-draft-model=<model>] [--hf_overrides=<json>] [--gpu-memory-utilization=<float>] [--mode=<mode>] [--think] MODEL",
		Short:  "Configure runtime options for a model",
		Hidden: true,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf(
					"Exactly one model must be specified, got %d: %v\n\n"+
						"See 'docker model configure --help' for more information",
					len(args), args)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			model := args[0]
			opts, err := flags.BuildConfigureRequest(model)
			if err != nil {
				return err
			}
			return desktopClient.ConfigureBackend(opts)
		},
		ValidArgsFunction: completion.ModelNames(getDesktopClient, -1),
	}

	flags.RegisterFlags(c)
	return c
}
