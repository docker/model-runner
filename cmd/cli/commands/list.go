package commands

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strconv"
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
	c := &cobra.Command{
		Use:     "list [OPTIONS]",
		Aliases: []string{"ls"},
		Short:   "List the models pulled to your local environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			if openai && quiet {
				return fmt.Errorf("--quiet flag cannot be used with --openai flag or OpenAI backend")
			}

			// If we're doing an automatic install, only show the installation
			// status if it won't corrupt machine-readable output.
			var standaloneInstallPrinter standalone.StatusPrinter
			if !jsonFormat && !openai && !quiet {
				standaloneInstallPrinter = asPrinter(cmd)
			}
			if _, err := ensureStandaloneRunnerAvailable(cmd.Context(), standaloneInstallPrinter, false); err != nil {
				return fmt.Errorf("unable to initialize standalone model runner: %w", err)
			}
			var modelFilter string
			if len(args) > 0 {
				modelFilter = args[0]
			}
			models, err := listModels(openai, desktopClient, quiet, jsonFormat, modelFilter)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), models)
			return nil
		},
		ValidArgsFunction: completion.ModelNamesAndTags(getDesktopClient, 1),
	}
	c.Flags().BoolVar(&jsonFormat, "json", false, "List models in a JSON format")
	c.Flags().BoolVar(&openai, "openai", false, "List models in an OpenAI format")
	c.Flags().BoolVarP(&quiet, "quiet", "q", false, "Only show model IDs")
	return c
}

func listModels(openai bool, desktopClient *desktop.Client, quiet bool, jsonFormat bool, modelFilter string) (string, error) {
	if openai {
		models, err := desktopClient.ListOpenAI()
		if err != nil {
			return "", handleClientError(err, "Failed to list models")
		}
		return formatter.ToStandardJSON(models)
	}
	models, err := desktopClient.List()
	if err != nil {
		return "", handleClientError(err, "Failed to list models")
	}

	if modelFilter != "" {
		// If filter doesn't contain '/', prepend default namespace 'ai/'
		if !strings.Contains(modelFilter, "/") {
			modelFilter = "ai/" + modelFilter
		}

		var filteredModels []dmrm.Model

		// Check if filter has a colon (i.e., includes a tag)
		hasColon := strings.Contains(modelFilter, ":")

		for _, m := range models {
			var matchingTags []string
			for _, tag := range m.Tags {
				if hasColon {
					// Filter includes a tag part - do exact match
					// Tags are stored in normalized format by the backend
					if tag == modelFilter {
						matchingTags = append(matchingTags, tag)
					}
				} else {
					// Filter has no colon - match repository name only (part before ':')
					repository, _, _ := strings.Cut(tag, ":")
					if repository == modelFilter {
						matchingTags = append(matchingTags, tag)
					}
				}
			}
			// Only include the model if at least one tag matched, and only include matching tags
			if len(matchingTags) > 0 {
				// Create a copy of the model with only the matching tags
				filteredModel := m
				filteredModel.Tags = matchingTags
				filteredModels = append(filteredModels, filteredModel)
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

func prettyPrintModels(models []dmrm.Model) string {
	type displayRow struct {
		displayName string
		tag         string
		model       dmrm.Model
	}

	var rows []displayRow

	for _, m := range models {
		if len(m.Tags) == 0 {
			rows = append(rows, displayRow{
				displayName: "<none>",
				tag:         "<none>",
				model:       m,
			})
			continue
		}

		for _, tag := range m.Tags {
			displayName := stripDefaultsFromModelName(tag)
			rows = append(rows, displayRow{
				displayName: displayName,
				tag:         tag,
				model:       m,
			})
		}
	}

	// Helper function to split display name into base and variant
	splitDisplayName := func(displayName string) (base, variant string) {
		if idx := strings.LastIndex(displayName, ":"); idx != -1 {
			return displayName[:idx], displayName[idx+1:]
		}
		return displayName, ""
	}

	// Sort all rows by display name
	sort.Slice(rows, func(i, j int) bool {
		displayI := rows[i].displayName
		displayJ := rows[j].displayName

		// Split on last ':' to get base name and variant
		baseI, variantI := splitDisplayName(displayI)
		baseJ, variantJ := splitDisplayName(displayJ)

		baseILower := strings.ToLower(baseI)
		baseJLower := strings.ToLower(baseJ)
		if baseILower != baseJLower {
			return baseILower < baseJLower
		}

		// If base names are equal, compare variants
		// Empty variants (no ':' in name) come first
		if variantI == "" && variantJ != "" {
			return true
		}
		if variantI != "" && variantJ == "" {
			return false
		}
		return strings.ToLower(variantI) < strings.ToLower(variantJ)
	})

	var buf bytes.Buffer
	table := tablewriter.NewWriter(&buf)

	table.SetHeader([]string{"MODEL NAME", "PARAMETERS", "QUANTIZATION", "ARCHITECTURE", "MODEL ID", "CREATED", "CONTEXT", "SIZE"})

	table.SetBorder(false)
	table.SetColumnSeparator("")
	table.SetHeaderLine(false)
	table.SetTablePadding("  ")
	table.SetNoWhiteSpace(true)

	table.SetColumnAlignment([]int{
		tablewriter.ALIGN_LEFT,  // MODEL
		tablewriter.ALIGN_LEFT,  // PARAMETERS
		tablewriter.ALIGN_LEFT,  // QUANTIZATION
		tablewriter.ALIGN_LEFT,  // ARCHITECTURE
		tablewriter.ALIGN_LEFT,  // MODEL ID
		tablewriter.ALIGN_LEFT,  // CREATED
		tablewriter.ALIGN_RIGHT, // CONTEXT
		tablewriter.ALIGN_LEFT,  // SIZE
	})
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)

	for _, row := range rows {
		appendRow(table, row.tag, row.model)
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
	contextSize := ""
	if model.Config.ContextSize != nil {
		contextSize = fmt.Sprintf("%d", *model.Config.ContextSize)
	} else if model.Config.GGUF != nil {
		if v, ok := model.Config.GGUF["llama.context_length"]; ok {
			if parsed, err := strconv.ParseUint(v, 10, 64); err == nil {
				contextSize = fmt.Sprintf("%d", parsed)
			} else {
				fmt.Fprintf(os.Stderr, "invalid context length %q for model %s: %v\n", v, model.ID, err)
			}
		}
	}

	table.Append([]string{
		displayTag,
		model.Config.Parameters,
		model.Config.Quantization,
		model.Config.Architecture,
		model.ID[7:19],
		units.HumanDuration(time.Since(time.Unix(model.Created, 0))) + " ago",
		contextSize,
		model.Config.Size,
	})
}
