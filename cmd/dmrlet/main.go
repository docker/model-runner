// dmrlet is a lightweight node agent for Docker Model Runner.
// It runs inference containers directly with zero YAML overhead.
package main

import (
	"fmt"
	"os"

	"github.com/docker/model-runner/cmd/dmrlet/commands"
)

func main() {
	if err := commands.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
