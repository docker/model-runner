//go:build !gateway

package commands

import (
	"errors"

	"github.com/spf13/cobra"
)

// newGatewayCmd returns a metadata-only stub used by the docs generator and
// any build that was compiled without -tags gateway (i.e. without the Rust
// static library).  The command is fully described so that 'make docs' can
// generate correct reference documentation, but it exits with an error if
// actually invoked.
func newGatewayCmd() *cobra.Command {
	var (
		config  string
		host    string
		port    uint16
		verbose bool
	)

	c := &cobra.Command{
		Use:   "gateway",
		Short: "Run an OpenAI-compatible LLM gateway",
		Long: `Run an OpenAI-compatible LLM gateway that routes requests to configured providers.

Supported providers include Docker Model Runner, Ollama, OpenAI, Anthropic,
Groq, Mistral, Azure OpenAI, and many more OpenAI-compatible endpoints.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("gateway is not available in this build; rebuild with 'make build-cli'")
		},
	}

	c.Flags().StringVarP(&config, "config", "c", "", "Path to the YAML configuration file")
	c.Flags().StringVar(&host, "host", "0.0.0.0", "Host address to bind to")
	c.Flags().Uint16VarP(&port, "port", "p", 4000, "Port to listen on")
	c.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose (debug) logging")
	_ = c.MarkFlagRequired("config")

	return c
}
