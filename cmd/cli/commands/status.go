package commands

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/docker/cli/cli-plugins/hooks"
	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/cmd/cli/pkg/standalone"
	"github.com/docker/model-runner/cmd/cli/pkg/types"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	var formatJson bool
	c := &cobra.Command{
		Use:   "status",
		Short: "Check if the Docker Model Runner is running",
		RunE: func(cmd *cobra.Command, args []string) error {
			runner, err := ensureStandaloneRunnerAvailable(cmd.Context(), asPrinter(cmd), false)
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
				return jsonStatus(asPrinter(cmd), runner, status, backendStatus)
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

func makeEndpoint(host string, port int) string {
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/v1/"
}

func jsonStatus(printer standalone.StatusPrinter, runner *standaloneRunner, status desktop.Status, backendStatus map[string]string) error {
	type Status struct {
		Running      bool              `json:"running"`
		Backends     map[string]string `json:"backends"`
		Kind         string            `json:"kind"`
		Endpoint     string            `json:"endpoint"`
		EndpointHost string            `json:"endpointHost"`
	}
	var endpoint, endpointHost string
	kind := modelRunner.EngineKind()
	switch kind {
	case types.ModelRunnerEngineKindDesktop:
		endpoint = "http://model-runner.docker.internal/v1/"
		endpointHost = modelRunner.URL("/v1/")
	case types.ModelRunnerEngineKindMobyManual:
		endpoint = modelRunner.URL("/v1/")
		endpointHost = endpoint
	case types.ModelRunnerEngineKindCloud:
		gatewayIP := "127.0.0.1"
		var gatewayPort uint16 = standalone.DefaultControllerPortCloud
		if runner != nil {
			if runner.gatewayIP != "" {
				gatewayIP = runner.gatewayIP
			}
			if runner.gatewayPort != 0 {
				gatewayPort = runner.gatewayPort
			}
		}
		endpoint = makeEndpoint(gatewayIP, int(gatewayPort))
		endpointHost = makeEndpoint("127.0.0.1", standalone.DefaultControllerPortCloud)
	case types.ModelRunnerEngineKindMoby:
		endpoint = makeEndpoint("host.docker.internal", standalone.DefaultControllerPortMoby)
		endpointHost = makeEndpoint("127.0.0.1", standalone.DefaultControllerPortMoby)
	default:
		return fmt.Errorf("unhandled engine kind: %v", kind)
	}
	s := Status{
		Running:      status.Running,
		Backends:     backendStatus,
		Kind:         kind.String(),
		Endpoint:     endpoint,
		EndpointHost: endpointHost,
	}
	marshal, err := json.Marshal(s)
	if err != nil {
		return err
	}
	printer.Println(string(marshal))
	return nil
}

var osExit = os.Exit
