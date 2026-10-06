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
	"github.com/docker/model-runner/pkg/distribution/types"
	dmrm "github.com/docker/model-runner/pkg/inference/models"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var jsonFormat, openai, quiet bool
	var openaiURL string
	c := &cobra.Command{
		Use:     "list [OPTIONS] [MODEL]",
		Aliases: []string{"ls"},
		Short:   "List the models pulled to your local environment",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if openai && quiet {
				return fmt.Errorf("--quiet flag cannot be used with --openai flag or OpenAI backend")
			}

			// Handle --openaiurl flag for external OpenAI endpoints
			if openaiURL != "" {
				if quiet {
					return fmt.Errorf("--quiet flag cannot be used with --openaiurl flag")
				}
				ctx, err := desktop.NewContextForOpenAI(openaiURL)
				if err != nil {
					return fmt.Errorf("invalid OpenAI URL: %w", err)
				}
				client := desktop.New(ctx)
				models, err := client.ListOpenAI()
				if err != nil {
					return handleClientError(err, "Failed to list models from OpenAI endpoint")
				}
				var modelFilter string
				if len(args) > 0 {
					modelFilter = args[0]
				}
				if modelFilter != "" {
					filtered := models.Data[:0]
					for _, m := range models.Data {
						if matchesModelFilter(m.ID, modelFilter) {
							filtered = append(filtered, m)
						}
					}
					models.Data = filtered
				}
				if jsonFormat {
					output, err := formatter.ToStandardJSON(models)
					if err != nil {
						return err
					}
					fmt.Fprint(cmd.OutOrStdout(), output)
					return nil
				}
				// Display in table format with only MODEL NAME populated
				fmt.Fprint(cmd.OutOrStdout(), prettyPrintOpenAIModels(models))
				return nil
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
	c.Flags().StringVar(&openaiURL, "openaiurl", "", "OpenAI-compatible API endpoint URL to list models from")
	return c
}

func normalizeModelFilter(filter string) string {
	if !strings.Contains(filter, "/") {
		return "ai/" + filter
	}
	return filter
}

func matchesModelFilter(tag, filter string) bool {
	if strings.Contains(filter, ":") {
		return tag == filter
	}
	repository, _, _ := strings.Cut(tag, ":")
	return repository == filter
}

func listModels(openai bool, desktopClient *desktop.Client, quiet bool, jsonFormat bool, modelFilter string) (string, error) {
	if openai {
		models, err := desktopClient.ListOpenAI()
		if err != nil {
			return "", handleClientError(err, "Failed to list models")
		}
		if modelFilter != "" {
			filter := normalizeModelFilter(modelFilter)
			filtered := models.Data[:0]
			for _, m := range models.Data {
				if matchesModelFilter(m.ID, filter) {
					filtered = append(filtered, m)
				}
			}
			models.Data = filtered
		}
		return formatter.ToStandardJSON(models)
	}

	models, err := desktopClient.List()
	if err != nil {
		return "", handleClientError(err, "Failed to list models")
	}
	if modelFilter != "" {
		filter := normalizeModelFilter(modelFilter)
		var filteredModels []dmrm.Model

		for _, m := range models {
			var matchingTags []string
			for _, tag := range m.Tags {
				if matchesModelFilter(tag, filter) {
					matchingTags = append(matchingTags, tag)
				}
			}
			if len(matchingTags) > 0 {
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
	table := newTable(&buf)
	table.Header([]string{"MODEL NAME", "PARAMETERS", "QUANTIZATION", "ARCHITECTURE", "MODEL ID", "CREATED", "CONTEXT", "SIZE"})

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
	if model.Config.GetContextSize() != nil {
		contextSize = fmt.Sprintf("%d", *model.Config.GetContextSize())
	} else if dockerConfig, ok := model.Config.(*types.Config); ok && dockerConfig.GGUF != nil {
		if v, ok := dockerConfig.GGUF["llama.context_length"]; ok {
			if parsed, err := strconv.ParseUint(v, 10, 64); err == nil {
				contextSize = fmt.Sprintf("%d", parsed)
			} else {
				fmt.Fprintf(os.Stderr, "invalid context length %q for model %s: %v\n", v, model.ID, err)
			}
		}
	}

	table.Append([]string{
		displayTag,
		model.Config.GetParameters(),
		model.Config.GetQuantization(),
		model.Config.GetArchitecture(),
		model.ID[7:19],
		units.HumanDuration(time.Since(time.Unix(model.Created, 0))) + " ago",
		contextSize,
		model.Config.GetSize(),
	})
}

// prettyPrintOpenAIModels formats OpenAI model list in table format with only MODEL NAME populated
func prettyPrintOpenAIModels(models dmrm.OpenAIModelList) string {
	// Sort models by ID
	sort.Slice(models.Data, func(i, j int) bool {
		return strings.ToLower(models.Data[i].ID) < strings.ToLower(models.Data[j].ID)
	})

	var buf bytes.Buffer
	table := newTable(&buf)
	table.Header([]string{"MODEL NAME", "CREATED"})
	for _, model := range models.Data {
		table.Append([]string{
			model.ID,
			units.HumanDuration(time.Since(time.Unix(model.Created, 0))) + " ago",
		})
	}

	table.Render()
	return buf.String()
}
