// dmrlet is a lightweight node agent for Docker Model Runner.
// It runs inference containers directly with zero YAML overhead.
package main

import (
	"os"

	"github.com/docker/model-runner/cmd/dmrlet/commands"
)

func main() {
	if err := commands.Execute(); err != nil {
		os.Exit(1)
	}
}
