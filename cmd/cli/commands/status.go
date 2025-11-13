package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/docker/model-runner/cmd/cli/pkg/types"

	"github.com/docker/cli/cli-plugins/hooks"
	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	var formatJson bool
	c := &cobra.Command{
		Use:   "status",
		Short: "Check if the Docker Model Runner is running",
		RunE: func(cmd *cobra.Command, args []string) error {
			standalone, err := ensureStandaloneRunnerAvailable(cmd.Context(), asPrinter(cmd), false)
			if err != nil {
				return fmt.Errorf("unable to initialize standalone model runner: %w", err)
			}
			status := desktopClient.Status()
			if status.Error != nil {
				return handleClientError(status.Error, "Failed to get Docker Model Runner status")
			}

			if len(status.Status) == 0 {
				status.Status = []byte("{}")
			}

			var backendStatus map[string]string
			if err := json.Unmarshal(status.Status, &backendStatus); err != nil {
				cmd.PrintErrln(fmt.Errorf("failed to parse status response: %w", err))
			}

			if formatJson {
				return jsonStatus(standalone, status, backendStatus)
			} else {
				textStatus(cmd, status, backendStatus)
			}

			return nil
		},
		ValidArgsFunction: completion.NoComplete,
	}
	c.Flags().BoolVar(&formatJson, "json", false, "Format output in JSON")
	return c
}

func textStatus(cmd *cobra.Command, status desktop.Status, backendStatus map[string]string) {
	if status.Running {
		cmd.Println("Docker Model Runner is running")
		cmd.Println("\nStatus:")
		for b, s := range backendStatus {
			if s != "not running" {
				cmd.Println(b+":", s)
			}
		}
	} else {
		cmd.Println("Docker Model Runner is not running")
		hooks.PrintNextSteps(cmd.OutOrStdout(), []string{enableViaCLI, enableViaGUI})
		osExit(1)
	}
}

func jsonStatus(standalone *standaloneRunner, status desktop.Status, backendStatus map[string]string) error {
	type Status struct {
		Running  bool              `json:"running"`
		Backends map[string]string `json:"backends"`
		Endpoint string            `json:"endpoint"`
	}
	var endpoint string
	kind := modelRunner.EngineKind()
	switch kind {
	case types.ModelRunnerEngineKindDesktop:
		endpoint = "http://model-runner.docker.internal/engines/v1/"
	case types.ModelRunnerEngineKindMobyManual:
		endpoint = modelRunner.URL("/engines/v1/")
	case types.ModelRunnerEngineKindCloud:
		fallthrough
	case types.ModelRunnerEngineKindMoby:
		if standalone.gatewayIP == "" {
			standalone.gatewayIP = "127.0.0.1"
		}

		if standalone.gatewayPort == 0 {
			standalone.gatewayPort = 12434
		}

		endpoint = fmt.Sprintf("http://%s:%d/engines/v1/", standalone.gatewayIP, standalone.gatewayPort)
	default:
		return fmt.Errorf("unhandled engine kind: %v", kind)
	}
	s := Status{
		Running:  status.Running,
		Backends: backendStatus,
		Endpoint: endpoint,
	}
	marshal, err := json.Marshal(s)
	if err != nil {
		return err
	}
	fmt.Println(string(marshal))
	return nil
}

var osExit = os.Exit
