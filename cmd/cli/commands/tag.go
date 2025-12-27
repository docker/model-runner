package commands

import (
	"fmt"
	"strings"

	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/pkg/distribution/oci/reference"
	"github.com/docker/model-runner/pkg/distribution/registry"
	"github.com/spf13/cobra"
)

func newTagCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "tag SOURCE TARGET",
		Short: "Tag a model",
		Args:  requireExactArgs(2, "tag", "SOURCE TARGET"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return tagModel(cmd, desktopClient, args[0], args[1])
		},
		ValidArgsFunction: completion.ModelNames(getDesktopClient, 1),
	}
	return c
}

func tagModel(cmd *cobra.Command, desktopClient *desktop.Client, source, target string) error {
	// Ensure tag is valid
	tag, err := reference.NewTag(target, registry.GetDefaultRegistryOptions()...)
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
func parseRepo(tag *reference.Tag) string {
	return strings.TrimSuffix(tag.String(), ":"+tag.TagStr())
}
