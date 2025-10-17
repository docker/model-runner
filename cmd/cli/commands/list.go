package commands

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/docker/go-units"
	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/cmd/cli/commands/formatter"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/cmd/cli/pkg/standalone"
	dmrm "github.com/docker/model-runner/pkg/inference/models"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var jsonFormat, openai, quiet bool
	var backend string
	var host string
	var port int
	var customURL string
	var urlAlias string

	c := &cobra.Command{
		Use:     "list [OPTIONS]",
		Aliases: []string{"ls"},
		Short:   "List the models pulled to your local environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve server URL from flags
			serverURL, useOpenAI, apiKey, err := resolveServerURL(host, customURL, urlAlias, port)
			if err != nil {
				return err
			}

			// Override model runner context if server URL is specified
			if serverURL != "" {
				if err := overrideModelRunnerContextFromURL(serverURL, useOpenAI); err != nil {
					return err
				}
			} else if host != "" || port != 0 {
				if err := overrideModelRunnerContext(host, port); err != nil {
					return err
				}
			}

			// Validate backend if specified
			if backend != "" {
				if err := validateBackend(backend); err != nil {
					return err
				}
			}

			// If using OpenAI-compatible endpoints, set backend to "openai"
			// Note: We don't automatically set openai=true here because that controls output format
			// Users need to explicitly pass --openai flag for OpenAI JSON format output
			if useOpenAI {
				if backend == "" {
					backend = "openai"
				}
			}

			if openai && quiet {
				return fmt.Errorf("--quiet flag cannot be used with --openai flag")
			}

			// Validate API key for OpenAI backend (legacy backend flag)
			if backend != "" && apiKey == "" {
				apiKey, err = ensureAPIKey(backend)
				if err != nil {
					return err
				}
			}

			// If we're doing an automatic install, only show the installation
			// status if it won't corrupt machine-readable output.
			var standaloneInstallPrinter standalone.StatusPrinter
			if !jsonFormat && !openai && !quiet && backend == "" {
				standaloneInstallPrinter = cmd
			}
			if _, err := ensureStandaloneRunnerAvailable(cmd.Context(), standaloneInstallPrinter); err != nil {
				return fmt.Errorf("unable to initialize standalone model runner: %w", err)
			}
			var modelFilter string
			if len(args) > 0 {
				modelFilter = args[0]
			}
			models, err := listModels(openai, backend, desktopClient, quiet, jsonFormat, apiKey, modelFilter)
			if err != nil {
				return err
			}
			cmd.Print(models)
			return nil
		},
		ValidArgsFunction: completion.ModelNamesAndTags(getDesktopClient, 1),
	}
	c.Flags().BoolVar(&jsonFormat, "json", false, "List models in a JSON format")
	c.Flags().BoolVar(&openai, "openai", false, "List models in an OpenAI format")
	c.Flags().BoolVarP(&quiet, "quiet", "q", false, "Only show model IDs")
	c.Flags().StringVar(&backend, "backend", "", fmt.Sprintf("Specify the backend to use (%s)", ValidBackendsKeys()))
	c.Flags().MarkHidden("backend")

	// Server connection flags
	c.Flags().StringVar(&host, "host", "", "Host address to bind Docker Model Runner (default \"127.0.0.1\")")
	c.Flags().IntVar(&port, "port", 0, "Docker container port for Docker Model Runner (default: 12434)")
	c.Flags().StringVar(&customURL, "url", "", "Base URL for the model API")
	c.Flags().StringVar(&urlAlias, "url-alias", "", "Use openai alias server output (llamacpp|ollama|openrouter)")

	return c
}

func listModels(openai bool, backend string, desktopClient *desktop.Client, quiet bool, jsonFormat bool, apiKey string, modelFilter string) (string, error) {
	if backend == "openai" {
		models, err := desktopClient.ListOpenAI(backend, apiKey)
		if err != nil {
			err = handleClientError(err, "Failed to list models")
			return "", handleNotRunningError(err)
		}

		// Support different output formats
		if openai || jsonFormat {
			return formatter.ToStandardJSON(models)
		}
		if quiet {
			var modelIDs string
			for _, m := range models.Data {
				modelIDs += fmt.Sprintf("%s\n", m.ID)
			}
			return modelIDs, nil
		}
		return prettyPrintOpenAIModels(models), nil
	}
	models, err := desktopClient.List()
	if err != nil {
		err = handleClientError(err, "Failed to list models")
		return "", handleNotRunningError(err)
	}

	if modelFilter != "" {
		// Normalize the filter to match stored model names
		normalizedFilter := dmrm.NormalizeModelName(modelFilter)
		var filteredModels []dmrm.Model
		for _, m := range models {
			hasMatchingTag := false
			for _, tag := range m.Tags {
				if tag == normalizedFilter {
					hasMatchingTag = true
					break
				}
				// Also check without the tag part
				modelName, _, _ := strings.Cut(tag, ":")
				filterName, _, _ := strings.Cut(normalizedFilter, ":")
				if modelName == filterName {
					hasMatchingTag = true
					break
				}
			}
			if hasMatchingTag {
				filteredModels = append(filteredModels, m)
			}
		}
		models = filteredModels
	}

	if jsonFormat {
		return formatter.ToStandardJSON(models)
	}
	if quiet {
		var modelIDs string
		for _, m := range models {
			if len(m.ID) < 19 {
				fmt.Fprintf(os.Stderr, "invalid image ID for model: %v\n", m)
				continue
			}
			modelIDs += fmt.Sprintf("%s\n", m.ID[7:19])
		}
		return modelIDs, nil
	}
	return prettyPrintModels(models), nil
}

func prettyPrintOpenAIModels(modelList dmrm.OpenAIModelList) string {
	var buf bytes.Buffer
	table := tablewriter.NewWriter(&buf)

	table.SetHeader([]string{"MODEL NAME", "CREATED"})

	table.SetBorder(false)
	table.SetColumnSeparator("")
	table.SetHeaderLine(false)
	table.SetTablePadding("  ")
	table.SetNoWhiteSpace(true)

	table.SetColumnAlignment([]int{
		tablewriter.ALIGN_LEFT, // MODEL NAME
		tablewriter.ALIGN_LEFT, // CREATED
	})
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)

	for _, m := range modelList.Data {
		table.Append([]string{
			m.ID,
			units.HumanDuration(time.Since(time.Unix(m.Created, 0))) + " ago",
		})
	}

	table.Render()
	return buf.String()
}

func prettyPrintModels(models []dmrm.Model) string {
	var buf bytes.Buffer
	table := tablewriter.NewWriter(&buf)

	table.SetHeader([]string{"MODEL NAME", "PARAMETERS", "QUANTIZATION", "ARCHITECTURE", "MODEL ID", "CREATED", "SIZE"})

	table.SetBorder(false)
	table.SetColumnSeparator("")
	table.SetHeaderLine(false)
	table.SetTablePadding("  ")
	table.SetNoWhiteSpace(true)

	table.SetColumnAlignment([]int{
		tablewriter.ALIGN_LEFT, // MODEL
		tablewriter.ALIGN_LEFT, // PARAMETERS
		tablewriter.ALIGN_LEFT, // QUANTIZATION
		tablewriter.ALIGN_LEFT, // ARCHITECTURE
		tablewriter.ALIGN_LEFT, // MODEL ID
		tablewriter.ALIGN_LEFT, // CREATED
		tablewriter.ALIGN_LEFT, // SIZE
	})
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)

	for _, m := range models {
		if len(m.Tags) == 0 {
			appendRow(table, "<none>", m)
			continue
		}
		for _, tag := range m.Tags {
			appendRow(table, tag, m)
		}
	}

	table.Render()
	return buf.String()
}

func appendRow(table *tablewriter.Table, tag string, model dmrm.Model) {
	if len(model.ID) < 19 {
		fmt.Fprintf(os.Stderr, "invalid model ID for model: %v\n", model)
		return
	}
	// Strip default "ai/" prefix and ":latest" tag for display
	displayTag := stripDefaultsFromModelName(tag)
	table.Append([]string{
		displayTag,
		model.Config.Parameters,
		model.Config.Quantization,
		model.Config.Architecture,
		model.ID[7:19],
		units.HumanDuration(time.Since(time.Unix(model.Created, 0))) + " ago",
		model.Config.Size,
	})
}
