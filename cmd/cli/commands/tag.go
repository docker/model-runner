package commands

import (
	"fmt"
	"strings"

	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/spf13/cobra"
)

func newTagCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "tag SOURCE TARGET",
		Short: "Tag a model",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return fmt.Errorf(
					"'docker model tag' requires 2 arguments.\n\n" +
						"Usage:  docker model tag SOURCE TARGET\n\n" +
						"See 'docker model tag --help' for more information",
				)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := ensureStandaloneRunnerAvailable(cmd.Context(), cmd); err != nil {
				return fmt.Errorf("unable to initialize standalone model runner: %w", err)
			}
			return tagModel(cmd, desktopClient, args[0], args[1])
		},
		ValidArgsFunction: completion.ModelNames(getDesktopClient, 1),
	}
	return c
}

func tagModel(cmd *cobra.Command, desktopClient *desktop.Client, source, target string) error {
	// Normalize source model name to add default org and tag if missing
	source = models.NormalizeModelName(source)
	// Normalize target model name to add default org and tag if missing
	target = models.NormalizeModelName(target)
	// Ensure tag is valid
	tag, err := name.NewTag(target)
	if err != nil {
		return fmt.Errorf("invalid tag: %w", err)
	}
	// Make tag request with model runner client
	if err := desktopClient.Tag(source, parseRepo(tag), tag.TagStr()); err != nil {
		return fmt.Errorf("failed to tag model: %w", err)
	}
	cmd.Printf("Model %q tagged successfully with %q\n", source, target)
	return nil
}

// parseRepo returns the repo portion of the original target string. It does not include implicit
// index.docker.io when the registry is omitted.
func parseRepo(tag name.Tag) string {
	return strings.TrimSuffix(tag.String(), ":"+tag.TagStr())
}
