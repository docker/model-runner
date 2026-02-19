package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"

	"github.com/docker/cli/cli-plugins/hooks"
	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/cmd/cli/pkg/standalone"
	"github.com/docker/model-runner/cmd/cli/pkg/types"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	var formatJson bool
	c := &cobra.Command{
		Use:   "status",
		Short: "Check if the Docker Model Runner is running",
		RunE: func(cmd *cobra.Command, args []string) error {
			runner, err := getStandaloneRunner(cmd.Context())
			if err != nil {
				return fmt.Errorf("unable to get standalone model runner info: %w", err)
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
		cmd.Println()
		cmd.Print(backendStatusTable(backendStatus))
	} else {
		cmd.Println("Docker Model Runner is not running")
		hooks.PrintNextSteps(cmd.OutOrStdout(), []string{enableViaCLI, enableViaGUI})
		osExit(1)
	}
}

func backendStatusTable(backendStatus map[string]string) string {
	var buf bytes.Buffer
	table := newTable(&buf)
	table.Header([]string{"BACKEND", "STATUS", "DETAILS"})

	type backendInfo struct {
		name       string
		statusType string
		details    string
		sortOrder  int
	}

	backends := make([]backendInfo, 0, len(backendStatus))
	for name, statusText := range backendStatus {
		statusType, details := inference.ParseStatus(statusText)

		// Assign sort order: Running < Error < Not Installed < Installing
		sortOrder := 4
		switch statusType {
		case inference.StatusRunning:
			sortOrder = 0
		case inference.StatusError:
			sortOrder = 1
		case inference.StatusNotInstalled:
			sortOrder = 2
		case inference.StatusInstalling:
			sortOrder = 3
		}

		backends = append(backends, backendInfo{
			name:       name,
			statusType: statusType,
			details:    details,
			sortOrder:  sortOrder,
		})
	}

	sort.Slice(backends, func(i, j int) bool {
		if backends[i].sortOrder != backends[j].sortOrder {
			return backends[i].sortOrder < backends[j].sortOrder
		}
		return backends[i].name < backends[j].name
	})

	for _, backend := range backends {
		table.Append([]string{backend.name, backend.statusType, backend.details})
	}

	table.Render()
	return buf.String()
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
		gatewayIP := "host.docker.internal"
		if runner != nil && runner.gatewayIP != "" {
			gatewayIP = runner.gatewayIP
		}
		endpoint = makeEndpoint(gatewayIP, standalone.DefaultControllerPortMoby)
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
