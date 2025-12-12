package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"

	"github.com/docker/model-runner/cmd/cli/pkg/types"
	"github.com/spf13/pflag"

	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/backends/llamacpp"
	dmrm "github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/inference/scheduling"
	"github.com/spf13/cobra"
)

func newComposeCmd() *cobra.Command {

	c := &cobra.Command{
		Use: "compose EVENT",
	}
	upCmd := newUpCommand()
	downCmd := newDownCommand()
	c.AddCommand(upCmd, downCmd, newMetadataCommand(upCmd, downCmd))
	c.Hidden = true
	c.PersistentFlags().String("project-name", "", "compose project name") // unused by model

	return c
}

func newUpCommand() *cobra.Command {
	var models []string
	var ctxSize int64
	var backend string
	var draftModel string
	var numTokens int
	var minAcceptanceRate float64
	c := &cobra.Command{
		Use: "up",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(models) == 0 {
				err := errors.New("options.model is required")
				_ = sendError(err.Error())
				return err
			}

			sendInfo("Initializing model runner...")
			kind := modelRunner.EngineKind()
			standalone, err := ensureStandaloneRunnerAvailable(cmd.Context(), nil, false)
			if err != nil {
				_ = sendErrorf("Failed to initialize standalone model runner: %v", err)
				return fmt.Errorf("Failed to initialize standalone model runner: %w", err)
			} else if ((kind == types.ModelRunnerEngineKindMoby || kind == types.ModelRunnerEngineKindCloud) &&
				standalone == nil) ||
				(standalone != nil && (standalone.gatewayIP == "" || standalone.gatewayPort == 0)) {
				return errors.New("unable to determine standalone runner endpoint")
			}

			if err := downloadModelsOnlyIfNotFound(desktopClient, models); err != nil {
				return err
			}

			if ctxSize > 0 {
				sendInfo(fmt.Sprintf("Setting context size to %d", ctxSize))
			}

			// Build speculative config if any speculative flags are set
			var speculativeConfig *inference.SpeculativeDecodingConfig
			if draftModel != "" || numTokens > 0 || minAcceptanceRate > 0 {
				speculativeConfig = &inference.SpeculativeDecodingConfig{
					DraftModel:        draftModel,
					NumTokens:         numTokens,
					MinAcceptanceRate: minAcceptanceRate,
				}
				sendInfo(fmt.Sprintf("Enabling speculative decoding with draft model: %s", draftModel))
			}

			for _, model := range models {
				size := int32(ctxSize)
				if err := desktopClient.ConfigureBackend(scheduling.ConfigureRequest{
					Model: model,
					BackendConfiguration: inference.BackendConfiguration{
						ContextSize: &size,
						Speculative: speculativeConfig,
					},
				}); err != nil {
					configErrFmtString := "failed to configure backend for model %s with context-size %d"
					_ = sendErrorf(configErrFmtString+": %v", model, ctxSize, err)
					return fmt.Errorf(configErrFmtString+": %w", model, ctxSize, err)
				}
				sendInfo("Successfully configured backend for model " + model)
			}

			switch kind {
			case types.ModelRunnerEngineKindDesktop:
				_ = setenv("URL", "http://model-runner.docker.internal/engines/v1/")
			case types.ModelRunnerEngineKindMobyManual:
				_ = setenv("URL", modelRunner.URL("/engines/v1/"))
			case types.ModelRunnerEngineKindCloud:
				fallthrough
			case types.ModelRunnerEngineKindMoby:
				_ = setenv("URL", "http://"+net.JoinHostPort(standalone.gatewayIP, strconv.Itoa(int(standalone.gatewayPort)))+"/engines/v1/")
			default:
				return fmt.Errorf("unhandled engine kind: %v", kind)
			}
			return nil
		},
	}
	c.Flags().StringArrayVar(&models, "model", nil, "model to use")
	c.Flags().Int64Var(&ctxSize, "context-size", -1, "context size for the model")
	c.Flags().StringVar(&backend, "backend", llamacpp.Name, "inference backend to use")
	c.Flags().StringVar(&draftModel, "speculative-draft-model", "", "draft model for speculative decoding")
	c.Flags().IntVar(&numTokens, "speculative-num-tokens", 0, "number of tokens to predict speculatively")
	c.Flags().Float64Var(&minAcceptanceRate, "speculative-min-acceptance-rate", 0, "minimum acceptance rate for speculative decoding")
	_ = c.MarkFlagRequired("model")
	return c
}

func newDownCommand() *cobra.Command {
	c := &cobra.Command{
		Use: "down",
		RunE: func(cmd *cobra.Command, args []string) error {
			// No required cleanup on down
			return nil
		},
	}
	return c
}

func newMetadataCommand(upCmd, downCmd *cobra.Command) *cobra.Command {
	c := &cobra.Command{
		Use:   "metadata",
		Short: "Metadata for Docker Compose",
		RunE: func(cmd *cobra.Command, args []string) error {
			providerMetadata := ProviderMetadata{
				Description: "Docker Model Runner",
			}
			providerMetadata.Up = commandParameters(upCmd)
			providerMetadata.Down = commandParameters(downCmd)

			jsonMetadata, err := json.Marshal(providerMetadata)
			if err != nil {
				return err
			}
			fmt.Print(string(jsonMetadata))
			return nil
		},
	}
	return c
}

func downloadModelsOnlyIfNotFound(desktopClient *desktop.Client, models []string) error {
	modelsDownloaded, err := desktopClient.List()
	if err != nil {
		_ = sendErrorf("Failed to get models list: %v", err)
		return err
	}
	for _, model := range models {
		// Download the model if not already present in the local model store
		if !slices.ContainsFunc(modelsDownloaded, func(m dmrm.Model) bool {
			if model == m.ID {
				return true
			}
			for _, tag := range m.Tags {
				if tag == model {
					return true
				}
			}
			return false
		}) {
			printer := desktop.NewSimplePrinter(func(s string) {
				_ = sendInfo(s)
			})
			_, _, err = desktopClient.Pull(model, printer)
			if err != nil {
				_ = sendErrorf("Failed to pull model: %v", err)
				return fmt.Errorf("Failed to pull model: %w\n", err)
			}
		}

	}
	_ = setenv("MODEL", strings.Join(models, ","))
	return nil
}

type jsonMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func setenv(k, v string) error {
	marshal, err := json.Marshal(jsonMessage{
		Type:    "setenv",
		Message: fmt.Sprintf("%v=%v", k, v),
	})
	if err != nil {
		return err
	}
	_, err = fmt.Println(string(marshal))
	return err
}

func sendErrorf(message string, args ...any) error {
	return sendError(fmt.Sprintf(message, args...))
}

func sendError(message string) error {
	marshal, err := json.Marshal(jsonMessage{
		Type:    "error",
		Message: message,
	})
	if err != nil {
		return err
	}
	_, err = fmt.Println(string(marshal))
	return err
}

func sendInfo(s string) error {
	marshal, err := json.Marshal(jsonMessage{
		Type:    "info",
		Message: s,
	})
	if err != nil {
		return err
	}
	_, err = fmt.Println(string(marshal))
	return err
}

func commandParameters(cmd *cobra.Command) CommandMetadata {
	cmdMetadata := CommandMetadata{}
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		_, isRequired := f.Annotations[cobra.BashCompOneRequiredFlag]
		cmdMetadata.Parameters = append(cmdMetadata.Parameters, ParameterMetadata{
			Name:        f.Name,
			Description: f.Usage,
			Required:    isRequired,
			Type:        f.Value.Type(),
			Default:     f.DefValue,
		})
	})
	return cmdMetadata
}

type ProviderMetadata struct {
	Description string          `json:"description"`
	Up          CommandMetadata `json:"up"`
	Down        CommandMetadata `json:"down"`
}

type CommandMetadata struct {
	Parameters []ParameterMetadata `json:"parameters"`
}

type ParameterMetadata struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Type        string `json:"type"`
	Default     string `json:"default,omitempty"`
}
