package commands

import (
	"fmt"

	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/pkg/inference"

	"github.com/docker/model-runner/pkg/inference/scheduling"
	"github.com/spf13/cobra"
)

func newConfigureCmd() *cobra.Command {
	var opts scheduling.ConfigureRequest
	var draftModel string
	var numTokens int
	var minAcceptanceRate float64

	c := &cobra.Command{
		Use:    "configure [--context-size=<n>] [--speculative-draft-model=<model>] MODEL [-- <runtime-flags...>]",
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
			// Build the speculative config if any speculative flags are set
			if draftModel != "" || numTokens > 0 || minAcceptanceRate > 0 {
				opts.Speculative = &inference.SpeculativeDecodingConfig{
					DraftModel:        draftModel,
					NumTokens:         numTokens,
					MinAcceptanceRate: minAcceptanceRate,
				}
			}
			return desktopClient.ConfigureBackend(opts)
		},
		ValidArgsFunction: completion.ModelNames(getDesktopClient, -1),
	}

	c.Flags().Int64Var(&opts.ContextSize, "context-size", -1, "context size (in tokens)")
	c.Flags().StringVar(&draftModel, "speculative-draft-model", "", "draft model for speculative decoding")
	c.Flags().IntVar(&numTokens, "speculative-num-tokens", 0, "number of tokens to predict speculatively")
	c.Flags().Float64Var(&minAcceptanceRate, "speculative-min-acceptance-rate", 0, "minimum acceptance rate for speculative decoding")
	return c
}
