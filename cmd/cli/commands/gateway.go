//go:build gateway

package commands

/*
#cgo LDFLAGS: -L${SRCDIR}/gateway_lib -lgateway -lm
#cgo darwin LDFLAGS: -framework CoreFoundation -framework Security -framework SystemConfiguration
#cgo linux LDFLAGS: -lpthread -ldl -lssl -lcrypto

#include <stdlib.h>

extern int run_gateway(int argc, const char **argv);
*/
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/spf13/cobra"
)

func newGatewayCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "gateway",
		Short: "Run an OpenAI-compatible LLM gateway",
		Long: `Run an OpenAI-compatible LLM gateway that routes requests to configured providers.

Supported providers include Docker Model Runner, Ollama, OpenAI, Anthropic,
Groq, Mistral, Azure OpenAI, and many more OpenAI-compatible endpoints.`,
		// Pass all flags straight through to the Rust arg parser.
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Build a C argv: ["model-cli"] + args
			cArgs := make([]*C.char, 0, len(args)+1)
			cArgs = append(cArgs, C.CString("model-cli"))
			for _, a := range args {
				cArgs = append(cArgs, C.CString(a))
			}
			defer func() {
				for _, p := range cArgs {
					C.free(unsafe.Pointer(p))
				}
			}()

			rc := C.run_gateway(C.int(len(cArgs)), (**C.char)(unsafe.Pointer(&cArgs[0])))
			if rc != 0 {
				return fmt.Errorf("gateway exited with code %d", rc)
			}
			return nil
		},
	}
}
