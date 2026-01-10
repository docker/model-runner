package commands

import (
	"bytes"
	"fmt"

	"github.com/docker/model-runner/cmd/cli/commands/formatter"
	"github.com/docker/model-runner/cmd/cli/search"
	"github.com/spf13/cobra"
)

func newSearchCmd() *cobra.Command {
	var (
		limit      int
		source     string
		jsonFormat bool
	)

	c := &cobra.Command{
		Use:   "search [OPTIONS] [TERM]",
		Short: "Search for models on Docker Hub and HuggingFace",
		Long: `Search for models from Docker Hub (ai/ namespace) and HuggingFace.

When no search term is provided, lists all available models.
When a search term is provided, filters models by name/description.

Examples:
  docker model search                       # List available models from Docker Hub
  docker model search llama                 # Search for models containing "llama"
  docker model search --source=all          # Search both Docker Hub and HuggingFace
  docker model search --source=huggingface  # Only search HuggingFace
  docker model search --limit=50 phi        # Search with custom limit
  docker model search --json llama          # Output as JSON`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse the source
			sourceType, err := search.ParseSource(source)
			if err != nil {
				return err
			}

			// Get the search query
			var query string
			if len(args) > 0 {
				query = args[0]
			}

			// Create the search client
			client := search.NewAggregatedClient(sourceType, cmd.ErrOrStderr())

			// Perform the search
			opts := search.SearchOptions{
				Query: query,
				Limit: limit,
			}

			results, err := client.Search(cmd.Context(), opts)
			if err != nil {
				return fmt.Errorf("search failed: %w", err)
			}

			if len(results) == 0 {
				if query != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "No models found matching %q\n", query)
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), "No models found")
				}
				return nil
			}

			// Output results
			if jsonFormat {
				output, err := formatter.ToStandardJSON(results)
				if err != nil {
					return err
				}
				fmt.Fprint(cmd.OutOrStdout(), output)
				return nil
			}

			fmt.Fprint(cmd.OutOrStdout(), prettyPrintSearchResults(results))
			return nil
		},
	}

	c.Flags().IntVarP(&limit, "limit", "n", 32, "Maximum number of results to show")
	c.Flags().StringVar(&source, "source", "all", "Source to search: all, dockerhub, huggingface")
	c.Flags().BoolVar(&jsonFormat, "json", false, "Output results as JSON")

	return c
}

// prettyPrintSearchResults formats search results as a table
func prettyPrintSearchResults(results []search.SearchResult) string {
	var buf bytes.Buffer
	table := newTable(&buf)
	table.Header([]string{"NAME", "DESCRIPTION", "BACKEND", "DOWNLOADS", "STARS", "SOURCE"})

	for _, r := range results {
		name := r.Name
		if r.Source == search.HuggingFaceSourceName {
			name = "hf.co/" + r.Name
		}
		table.Append([]string{
			name,
			r.Description,
			r.Backend,
			formatCount(r.Downloads),
			formatCount(r.Stars),
			r.Source,
		})
	}

	table.Render()
	return buf.String()
}

// formatCount formats a number in a human-readable way (e.g., 1.2M, 45K)
func formatCount(n int64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}
