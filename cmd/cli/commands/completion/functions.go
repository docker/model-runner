package completion

import (
	"strings"

	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/spf13/cobra"
)

func NoComplete(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// ModelNames offers completion for models present within the local store.
func ModelNames(desktopClient func() *desktop.Client, limit int) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// HACK: Invoke rootCmd's PersistentPreRunE, which is needed for context
		// detection and client initialization. This function isn't invoked
		// automatically on autocompletion paths.
		if err := cmd.Parent().PersistentPreRunE(cmd, args); err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		if limit > 0 && len(args) >= limit {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		models, err := desktopClient().List()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		var names []string
		for _, m := range models {
			names = append(names, m.Tags...)
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	}
}

// ModelNamesAndTags offers completion that matches the base model name along with its tags.
// If the model has multiple tags, match both the base model name and each tag.
func ModelNamesAndTags(desktopClient func() *desktop.Client, limit int) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// HACK: Invoke rootCmd's PersistentPreRunE, which is needed for context
		// detection and client initialization. This function isn't invoked
		// automatically on autocompletion paths.
		if err := cmd.Parent().PersistentPreRunE(cmd, args); err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		if limit > 0 && len(args) >= limit {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		models, err := desktopClient().List()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		var names []string

		modelNames := make(map[string]bool)
		modelTags := make(map[string][]string)

		for _, m := range models {
			for _, tag := range m.Tags {
				// Extract model name (everything before the first colon or the full tag if no colon).
				modelName, _, _ := strings.Cut(tag, ":")
				modelNames[modelName] = true
				modelTags[modelName] = append(modelTags[modelName], tag)
			}
		}

		for name := range modelNames {
			// If model has multiple tags, suggest the base model name and all specific tags.
			if len(modelTags[name]) > 1 {
				names = append(names, name)
				for _, tag := range modelTags[name] {
					names = append(names, tag)
					// If this model doesn't have a tag, also add the :latest variant.
					if tag == name {
						names = append(names, tag+":latest")
					}
				}
			} else {
				// If only one tag, just suggest that tag to avoid duplication.
				names = append(names, modelTags[name][0])
			}
		}

		return names, cobra.ShellCompDirectiveNoSpace
	}
}
