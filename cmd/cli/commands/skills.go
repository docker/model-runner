package commands

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/spf13/cobra"
)

//go:embed skills/*
var skillsFS embed.FS

type skillsOptions struct {
	codex    bool
	claude   bool
	opencode bool
	dest     string
	force    bool
}

func newSkillsCmd() *cobra.Command {
	opts := &skillsOptions{}

	c := &cobra.Command{
		Use:   "skills",
		Short: "Install Docker Model Runner skills for AI coding assistants",
		Long: `Install Docker Model Runner skills for AI coding assistants.

Skills are configuration files that help AI coding assistants understand
how to use Docker Model Runner effectively for local model inference.

Supported targets:
  --codex     Install to ~/.codex/skills (OpenAI Codex CLI)
  --claude    Install to ~/.claude/skills (Claude Code)
  --opencode  Install to ~/.config/opencode/skills (OpenCode)
  --dest      Install to a custom directory

Example:
  docker model skills --claude
  docker model skills --codex --claude
  docker model skills --dest /path/to/skills`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSkills(cmd, opts)
		},
		ValidArgsFunction: completion.NoComplete,
	}

	c.Flags().BoolVar(&opts.codex, "codex", false, "Install skills for OpenAI Codex CLI (~/.codex/skills)")
	c.Flags().BoolVar(&opts.claude, "claude", false, "Install skills for Claude Code (~/.claude/skills)")
	c.Flags().BoolVar(&opts.opencode, "opencode", false, "Install skills for OpenCode (~/.config/opencode/skills)")
	c.Flags().StringVar(&opts.dest, "dest", "", "Install skills to a custom directory")
	c.Flags().BoolVarP(&opts.force, "force", "f", false, "Overwrite existing skills without prompting")

	return c
}

func runSkills(cmd *cobra.Command, opts *skillsOptions) error {
	// Collect target directories
	var targets []string
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	if opts.codex {
		targets = append(targets, filepath.Join(homeDir, ".codex", "skills"))
	}
	if opts.claude {
		targets = append(targets, filepath.Join(homeDir, ".claude", "skills"))
	}
	if opts.opencode {
		targets = append(targets, filepath.Join(homeDir, ".config", "opencode", "skills"))
	}
	if opts.dest != "" {
		targets = append(targets, opts.dest)
	}

	if len(targets) == 0 {
		return fmt.Errorf("no target specified. Use --codex, --claude, --opencode, or --dest")
	}

	// Install skills to each target
	for _, target := range targets {
		if err := installSkills(cmd, target, opts.force); err != nil {
			return fmt.Errorf("failed to install skills to %s: %w", target, err)
		}
		cmd.Printf("Installed Docker Model Runner skills to %s\n", target)
	}

	return nil
}

func installSkills(cmd *cobra.Command, targetDir string, force bool) error {
	// Walk through embedded skills directory
	return fs.WalkDir(skillsFS, "skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the root "skills" directory itself
		if path == "skills" {
			return nil
		}

		// Calculate the relative path from "skills/"
		relPath, err := filepath.Rel("skills", path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(targetDir, relPath)

		if d.IsDir() {
			// Create directory
			return os.MkdirAll(destPath, 0755)
		}

		// Check if file exists and handle force flag
		if _, err := os.Stat(destPath); err == nil && !force {
			return fmt.Errorf("file already exists: %s (use --force to overwrite)", destPath)
		}

		// Read the embedded file
		content, err := skillsFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read embedded file %s: %w", path, err)
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", destPath, err)
		}

		// Write the file
		if err := os.WriteFile(destPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", destPath, err)
		}

		return nil
	})
}
