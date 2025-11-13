package commands

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/spf13/cobra"
)

func newRequestsCmd() *cobra.Command {
	var model string
	var follow bool
	var includeExisting bool
	c := &cobra.Command{
		Use:   "requests [OPTIONS]",
		Short: "Fetch requests+responses from Docker Model Runner",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Make --include-existing only available when --follow is set.
			if includeExisting && !follow {
				return fmt.Errorf("--include-existing can only be used with --follow")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := ensureStandaloneRunnerAvailable(cmd.Context(), asPrinter(cmd), false); err != nil {
				return fmt.Errorf("unable to initialize standalone model runner: %w", err)
			}

			responseBody, cancel, err := desktopClient.Requests(model, follow, includeExisting)
			if err != nil {
				errMsg := "Failed to get requests"
				if model != "" {
					errMsg = errMsg + " for " + model
				}
				return handleClientError(err, errMsg)
			}
			defer cancel()

			if follow {
				scanner := bufio.NewScanner(responseBody)
				cmd.Println("Connected to request stream. Press Ctrl+C to stop.")
				var currentEvent string
				for scanner.Scan() {
					select {
					case <-cmd.Context().Done():
						return nil
					default:
					}
					line := scanner.Text()
					if strings.HasPrefix(line, "event: ") {
						currentEvent = strings.TrimPrefix(line, "event: ")
					} else if strings.HasPrefix(line, "data: ") &&
						(currentEvent == "new_request" || currentEvent == "existing_request") {
						data := strings.TrimPrefix(line, "data: ")
						cmd.Println(data)
					}
				}
				cmd.Println("Stream closed by server.")
			} else {
				body, err := io.ReadAll(responseBody)
				if err != nil {
					return fmt.Errorf("failed to read response body: %w", err)
				}
				cmd.Print(string(body))
			}

			return nil
		},
		ValidArgsFunction: completion.NoComplete,
	}
	c.Flags().BoolVarP(&follow, "follow", "f", false, "Follow requests stream")
	c.Flags().BoolVar(&includeExisting, "include-existing", false,
		"Include existing requests when starting to follow (only available with --follow)")
	c.Flags().StringVar(&model, "model", "", "Specify the model to filter requests")
	// Enable completion for the --model flag.
	_ = c.RegisterFlagCompletionFunc("model", completion.ModelNames(getDesktopClient, 1))
	return c
}
