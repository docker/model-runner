package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/cmd/cli/pkg/types"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/spf13/cobra"
)

func newCodeCmd() *cobra.Command {
	var backend string
	var aiderImage string

	const cmdArgs = "MODEL [PROMPT]"
	c := &cobra.Command{
		Use:   "code " + cmdArgs,
		Short: "Run aider in a container to edit code with AI assistance",
		Long: `Run aider in an ephemeral Docker container to edit code with AI assistance.

This command runs aider (https://github.com/paul-gauthier/aider) in a Docker container
that can interact with your local codebase and talk to Docker Model Runner.

The command must be run from the root of a Git repository. If no PROMPT is provided,
it will open your configured text editor (via EDITOR or VISUAL environment variables,
defaulting to vim) to compose a prompt, similar to how 'git commit' works.`,
		Example: `  # Open editor to compose prompt
  docker model code ai/smollm2

  # Provide prompt directly
  docker model code ai/smollm2 "Add error handling to the main function"

  # Use with a specific backend
  docker model code --backend openai gpt-4 "Refactor the authentication logic"`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Check if we're in a git repository
			gitCmd := exec.Command("git", "rev-parse", "--show-toplevel")
			if err := gitCmd.Run(); err != nil {
				return fmt.Errorf("must be run from within a git repository")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate backend if specified
			if backend != "" {
				if err := validateBackend(backend); err != nil {
					return err
				}
			}

			// Normalize model name to add default org and tag if missing
			model := models.NormalizeModelName(args[0])
			prompt := ""
			argsLen := len(args)
			if argsLen > 1 {
				prompt = strings.Join(args[1:], " ")
				if prompt == "" {
					if strings.TrimSpace(prompt) == "" {
						fmt.Fprintf(os.Stderr, "Aborting coding task due to empty commit message.\n")
						return nil
					}
				}
			}

			// If no prompt provided, open editor
			if prompt == "" {
				var err error
				prompt, err = getPromptFromEditor()
				if err != nil {
					return fmt.Errorf("failed to get prompt from editor: %w", err)
				}
				if strings.TrimSpace(prompt) == "" {
					fmt.Fprintf(os.Stderr, "Aborting coding task due to empty commit message.\n")
					return nil
				}
			}

			// Get the git repository root
			gitCmd := exec.Command("git", "rev-parse", "--show-toplevel")
			repoRootBytes, err := gitCmd.Output()
			if err != nil {
				return fmt.Errorf("failed to get repository root: %w", err)
			}
			repoRoot := strings.TrimSpace(string(repoRootBytes))

			// Get the model runner URL
			modelRunnerURL := getModelRunnerURL()

			// Ensure model is available
			if backend != "openai" {
				if _, err := ensureStandaloneRunnerAvailable(cmd.Context(), cmd); err != nil {
					return fmt.Errorf("unable to initialize standalone model runner: %w", err)
				}

				_, err := desktopClient.Inspect(model, false)
				if err != nil {
					cmd.Println("Unable to find model '" + model + "' locally. Pulling from the server.")
					if err := pullModel(cmd, desktopClient, model, false); err != nil {
						return err
					}
				}
			}

			// Run aider in Docker container
			return runAiderInContainer(cmd, aiderImage, repoRoot, model, prompt, modelRunnerURL)
		},
	}

	c.Args = func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("requires at least 1 argument: MODEL")
		}
		return nil
	}

	c.Flags().StringVar(&backend, "backend", "", "inference backend to use")
	c.Flags().StringVar(&aiderImage, "aider-image", "paulgauthier/aider", "Docker image to use for aider")

	return c
}

// getPromptFromEditor opens a text editor for the user to compose a prompt
func getPromptFromEditor() (string, error) {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vim"
	}

	// Create a temporary file for the prompt
	tmpFile, err := os.CreateTemp("", "model-code-prompt-*.txt")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	// Write instructions to the file
	instructions := `

# Please enter the commit message for your changes. Lines starting
# with '#' will be ignored, and an empty message aborts the commit
`
	if _, err := tmpFile.WriteString(instructions); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("failed to write instructions: %w", err)
	}
	tmpFile.Close()

	// Open the editor
	editorCmd := exec.Command(editor, tmpPath)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr

	if err := editorCmd.Run(); err != nil {
		return "", fmt.Errorf("editor exited with error: %w", err)
	}

	// Read the prompt from the file
	content, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", fmt.Errorf("failed to read prompt file: %w", err)
	}

	// Remove comment lines and trim
	lines := strings.Split(string(content), "\n")
	var promptLines []string
	for _, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			promptLines = append(promptLines, line)
		}
	}

	return strings.TrimSpace(strings.Join(promptLines, "\n")), nil
}

// getModelRunnerURL determines the Model Runner URL based on the context
func getModelRunnerURL() string {
	// Check if running in Docker Desktop environment
	if modelRunner != nil {
		kind := modelRunner.EngineKind()
		switch kind {
		case types.ModelRunnerEngineKindDesktop:
			return "http://model-runner.docker.internal/engines/v1/"
		case types.ModelRunnerEngineKindMobyManual:
			return modelRunner.URL("/engines/v1/")
		}
	}

	// Check for environment variable
	if url := os.Getenv("MODEL_RUNNER_HOST"); url != "" {
		// Ensure it ends with /engines/v1/ if not present
		if !strings.HasSuffix(url, "/") {
			url += "/"
		}
		if !strings.HasSuffix(url, "/engines/v1/") {
			url += "engines/v1/"
		}
		return url
	}

	// Default to localhost
	return "http://localhost:12434/engines/v1/"
}

// runAiderInContainer runs aider in a Docker container
func runAiderInContainer(cmd *cobra.Command, aiderImage, repoRoot, model, prompt, modelRunnerURL string) error {
        model = "openai/" + model

	// Build the aider command
	aiderArgs := []string{
		"run",
		"--rm",
		"-it",
		"-v", fmt.Sprintf("%s:/workspace", repoRoot),
		"-w", "/workspace",
		"-e", fmt.Sprintf("OPENAI_API_BASE=%s", modelRunnerURL),
		"-e", "OPENAI_API_KEY=dummy", // aider requires this but DMR doesn't need it
		"--entrypoint", "",
		"--network", "host", // Use host network to access model runner
		aiderImage,
		"aider",
		".",
		"--model", model,
		"--message", prompt,
		"--no-analytics", "--no-show-model-warnings", "--no-gitignore",
 		"--yes-always",
	}

	// Check if we're on macOS and adjust network settings
	if isDockerDesktop() {
		// Remove --network host and use Docker Desktop's DNS
		aiderArgs = removeElement(aiderArgs, "--network")
		aiderArgs = removeElement(aiderArgs, "host")
	}

	cmd.Printf("Running aider with model %s %s...\n", model, aiderArgs)

	dockerCmd := exec.Command("docker", aiderArgs...)
	dockerCmd.Stdin = os.Stdin
	dockerCmd.Stdout = os.Stdout
	dockerCmd.Stderr = os.Stderr

	if err := dockerCmd.Run(); err != nil {
		return fmt.Errorf("aider execution failed: %w", err)
	}

	return nil
}

// isDockerDesktop checks if we're running on Docker Desktop
func isDockerDesktop() bool {
	// Check for Docker Desktop indicators
	if modelRunner != nil {
		kind := modelRunner.EngineKind()
		return kind == types.ModelRunnerEngineKindDesktop
	}

	// Check for Docker Desktop on macOS/Windows
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".docker", "desktop")); err == nil {
		return true
	}

	return false
}

// removeElement removes all occurrences of a string from a slice
func removeElement(slice []string, element string) []string {
	result := []string{}
	for _, item := range slice {
		if item != element {
			result = append(result, item)
		}
	}
	return result
}

// getDesktopClient returns the desktop client (used by validation functions)
func getDesktopClientForCode() *desktop.Client {
	return desktopClient
}
