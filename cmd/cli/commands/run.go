package commands

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/charmbracelet/glamour"
	"github.com/docker/model-runner/cmd/cli/commands/completion"
	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/cmd/cli/readline"
	"github.com/fatih/color"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// readMultilineInput reads input from stdin, supporting both single-line and multiline input.
// For multiline input, it detects triple-quoted strings and shows continuation prompts.
func readMultilineInput(cmd *cobra.Command, scanner *bufio.Scanner) (string, error) {
	cmd.Print("> ")

	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("error reading input: %w", err)
		}
		return "", fmt.Errorf("EOF")
	}

	line := scanner.Text()

	// Check if this is the start of a multiline input (triple quotes)
	tripleQuoteStart := ""
	if strings.HasPrefix(line, `"""`) {
		tripleQuoteStart = `"""`
	} else if strings.HasPrefix(line, "'''") {
		tripleQuoteStart = "'''"
	}

	// If no triple quotes, return a single line
	if tripleQuoteStart == "" {
		return line, nil
	}

	// Check if the triple quotes are closed on the same line
	restOfLine := line[3:]
	if strings.HasSuffix(restOfLine, tripleQuoteStart) && len(restOfLine) >= 3 {
		// Complete multiline string on single line
		return line, nil
	}

	// Start collecting multiline input
	var multilineInput strings.Builder
	multilineInput.WriteString(line)
	multilineInput.WriteString("\n")

	// Continue reading lines until we find the closing triple quotes
	for {
		cmd.Print(". ")

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", fmt.Errorf("error reading input: %w", err)
			}
			return "", fmt.Errorf("unclosed multiline input (EOF)")
		}

		line = scanner.Text()
		multilineInput.WriteString(line)

		// Check if this line contains the closing triple quotes
		if strings.Contains(line, tripleQuoteStart) {
			// Found closing quotes, we're done
			break
		}

		multilineInput.WriteString("\n")
	}

	return multilineInput.String(), nil
}

// generateInteractiveWithReadline provides an enhanced interactive mode with readline support
func generateInteractiveWithReadline(cmd *cobra.Command, desktopClient *desktop.Client, model string) error {
	usage := func() {
		fmt.Fprintln(os.Stderr, "Available Commands:")
		fmt.Fprintln(os.Stderr, "  /set system     Set or update the system message")
		fmt.Fprintln(os.Stderr, "  /bye            Exit")
		fmt.Fprintln(os.Stderr, "  /?, /help       Help for a command")
		fmt.Fprintln(os.Stderr, "  /? shortcuts    Help for keyboard shortcuts")
		fmt.Fprintln(os.Stderr, "  /? files        Help for file inclusion with @ symbol")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, `Use """ to begin a multi-line message.`)
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "File Inclusion:")
		fmt.Fprintln(os.Stderr, "  Type @ followed by a filename to include its content in your prompt")
		fmt.Fprintln(os.Stderr, "  Examples: @README.md, @./src/main.go, @/path/to/file.txt")
		fmt.Fprintln(os.Stderr, "")
	}

	usageShortcuts := func() {
		fmt.Fprintln(os.Stderr, "Available keyboard shortcuts:")
		fmt.Fprintln(os.Stderr, "  Ctrl + a            Move to the beginning of the line (Home)")
		fmt.Fprintln(os.Stderr, "  Ctrl + e            Move to the end of the line (End)")
		fmt.Fprintln(os.Stderr, "   Alt + b            Move back (left) one word")
		fmt.Fprintln(os.Stderr, "   Alt + f            Move forward (right) one word")
		fmt.Fprintln(os.Stderr, "  Ctrl + k            Delete the sentence after the cursor")
		fmt.Fprintln(os.Stderr, "  Ctrl + u            Delete the sentence before the cursor")
		fmt.Fprintln(os.Stderr, "  Ctrl + w            Delete the word before the cursor")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  Ctrl + l            Clear the screen")
		fmt.Fprintln(os.Stderr, "  Ctrl + c            Stop the model from responding")
		fmt.Fprintln(os.Stderr, "  Ctrl + d            Exit (/bye)")
		fmt.Fprintln(os.Stderr, "")
	}

	usageFiles := func() {
		fmt.Fprintln(os.Stderr, "File Inclusion with @ symbol:")
		fmt.Fprintln(os.Stderr, "  Type @ followed by a filename to include its content in your prompt")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  Examples:")
		fmt.Fprintln(os.Stderr, "    @README.md          Include content of README.md from current directory")
		fmt.Fprintln(os.Stderr, "    @./src/main.go     Include content of main.go from ./src/ directory")
		fmt.Fprintln(os.Stderr, "    @/full/path/file   Include content of file using absolute path")
		fmt.Fprintln(os.Stderr, "    @\"file with spaces.txt\" Include content of file with spaces in name")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  The file content will be embedded in your prompt when you press Enter.")
		fmt.Fprintln(os.Stderr, "")
	}

	scanner, err := readline.New(readline.Prompt{
		Prompt:         "> ",
		AltPrompt:      ". ",
		Placeholder:    "Send a message (/? for help)",
		AltPlaceholder: `Use """ to end multi-line input`,
	})
	if err != nil {
		return err
	}

	// Disable history if the environment variable is set
	if os.Getenv("DOCKER_MODEL_NOHISTORY") != "" {
		scanner.HistoryDisable()
	}

	fmt.Print(readline.StartBracketedPaste)
	defer fmt.Printf(readline.EndBracketedPaste)

	var sb strings.Builder
	var multiline bool
	var conversationHistory []desktop.OpenAIChatMessage
	var systemPrompt string

	// Add a helper function to handle file inclusion when @ is pressed
	// We'll implement a basic version here that shows a message when @ is pressed

	for {
		line, err := scanner.Readline()
		switch {
		case errors.Is(err, io.EOF):
			fmt.Println()
			return nil
		case errors.Is(err, readline.ErrInterrupt):
			if line == "" {
				fmt.Println("\nUse Ctrl + d or /bye to exit.")
			}

			scanner.Prompt.UseAlt = false
			sb.Reset()

			continue
		case err != nil:
			return err
		}

		switch {
		case multiline:
			// check if there's a multiline terminating string
			before, ok := strings.CutSuffix(line, `"""`)
			sb.WriteString(before)
			if !ok {
				fmt.Fprintln(&sb)
				continue
			}

			multiline = false
			scanner.Prompt.UseAlt = false
		case strings.HasPrefix(line, `"""`):
			trimmed := strings.TrimPrefix(line, `"""`)
			content, ok := strings.CutSuffix(trimmed, `"""`)
			sb.WriteString(content)
			if !ok {
				// no multiline terminating string; need more input
				fmt.Fprintln(&sb)
				multiline = true
				scanner.Prompt.UseAlt = true
			}
		case scanner.Pasting:
			fmt.Fprintln(&sb, line)
			continue
		case strings.HasPrefix(line, "/help"), strings.HasPrefix(line, "/?"):
			args := strings.Fields(line)
			if len(args) > 1 {
				switch args[1] {
				case "shortcut", "shortcuts":
					usageShortcuts()
				case "file", "files":
					usageFiles()
				default:
					usage()
				}
			} else {
				usage()
			}
			continue
		case strings.HasPrefix(line, "/set system ") || line == "/set system":
			// Extract the system prompt text after "/set system "
			systemPrompt = strings.TrimPrefix(line, "/set system ")
			systemPrompt = strings.TrimSpace(systemPrompt)
			if systemPrompt == "" {
				fmt.Fprintln(os.Stderr, "Cleared system message.")
			} else {
				fmt.Fprintln(os.Stderr, "Set system message.")
			}
			continue
		case strings.HasPrefix(line, "/exit"), strings.HasPrefix(line, "/bye"):
			return nil
		case strings.HasPrefix(line, "/"):
			fmt.Printf("Unknown command '%s'. Type /? for help\n", strings.Fields(line)[0])
			continue
		default:
			sb.WriteString(line)
		}

		if sb.Len() > 0 && !multiline {
			userInput := sb.String()

			// Create a cancellable context for the chat request
			// This allows us to cancel the request if the user presses Ctrl+C during response generation
			chatCtx, cancelChat := context.WithCancel(cmd.Context())

			// Set up signal handler to cancel the context on Ctrl+C
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGINT)
			go func() {
				select {
				case <-sigChan:
					cancelChat()
				case <-chatCtx.Done():
					// Context cancelled, exit goroutine
				}
			}()

			// Build message history with system prompt prepended if set
			var messagesWithSystem []desktop.OpenAIChatMessage
			if systemPrompt == "" {
				messagesWithSystem = conversationHistory
			} else {
				messagesWithSystem = make([]desktop.OpenAIChatMessage, 1, 1+len(conversationHistory))
				messagesWithSystem[0] = desktop.OpenAIChatMessage{
					Role:    "system",
					Content: systemPrompt,
				}
				messagesWithSystem = append(messagesWithSystem, conversationHistory...)
			}

			assistantResponse, processedUserMessage, err := chatWithMarkdownContext(chatCtx, cmd, desktopClient, model, userInput, messagesWithSystem)

			// Clean up signal handler
			signal.Stop(sigChan)
			// Do not close sigChan to avoid race condition
			cancelChat()

			if err != nil {
				// Check if the error is due to context cancellation (Ctrl+C during response)
				if errors.Is(err, context.Canceled) {
					cmd.Println()
				} else {
					cmd.PrintErrln(handleClientError(err, "Failed to generate a response"))
				}
				sb.Reset()
				continue
			}

			// Add the processed user message and assistant response to conversation history.
			// Using the processed message ensures the history reflects exactly what the model
			// received (after file inclusions and image processing), not the raw user input.
			conversationHistory = append(conversationHistory, processedUserMessage)
			conversationHistory = append(conversationHistory, desktop.OpenAIChatMessage{
				Role:    "assistant",
				Content: assistantResponse,
			})

			cmd.Println()
			sb.Reset()
		}
	}
}

var (
	markdownRenderer *glamour.TermRenderer
	lastWidth        int
)

// StreamingMarkdownBuffer handles partial content and renders complete markdown blocks
type StreamingMarkdownBuffer struct {
	buffer       strings.Builder
	inCodeBlock  bool
	codeBlockEnd string // tracks the closing fence (``` or ```)
	lastFlush    int    // position of last flush
}

// NewStreamingMarkdownBuffer creates a new streaming markdown buffer
func NewStreamingMarkdownBuffer() *StreamingMarkdownBuffer {
	return &StreamingMarkdownBuffer{}
}

// AddContent adds new content to the buffer and returns any content that should be displayed
func (smb *StreamingMarkdownBuffer) AddContent(content string, shouldUseMarkdown bool) (string, error) {
	smb.buffer.WriteString(content)

	if !shouldUseMarkdown {
		// If not using markdown, just return the new content as-is
		result := content
		smb.lastFlush = smb.buffer.Len()
		return result, nil
	}

	return smb.processPartialMarkdown()
}

// processPartialMarkdown processes the buffer and returns content ready for display
func (smb *StreamingMarkdownBuffer) processPartialMarkdown() (string, error) {
	fullText := smb.buffer.String()

	// Look for code block start/end in the full text from our last position
	if !smb.inCodeBlock {
		// Check if we're entering a code block
		if idx := strings.Index(fullText[smb.lastFlush:], "```"); idx != -1 {
			// Found code block start
			beforeCodeBlock := fullText[smb.lastFlush : smb.lastFlush+idx]
			smb.inCodeBlock = true
			smb.codeBlockEnd = "```"

			// Stream everything before the code block as plain text
			smb.lastFlush = smb.lastFlush + idx
			return beforeCodeBlock, nil
		}

		// No code block found, stream all new content as plain text
		newContent := fullText[smb.lastFlush:]
		smb.lastFlush = smb.buffer.Len()
		return newContent, nil
	} else {
		// We're in a code block, look for the closing fence
		searchStart := smb.lastFlush
		if endIdx := strings.Index(fullText[searchStart:], smb.codeBlockEnd+"\n"); endIdx != -1 {
			// Found complete code block with newline after closing fence
			endPos := searchStart + endIdx + len(smb.codeBlockEnd) + 1
			codeBlockContent := fullText[smb.lastFlush:endPos]

			// Render the complete code block
			rendered, err := renderMarkdown(codeBlockContent)
			if err != nil {
				// Fallback to plain text
				smb.lastFlush = endPos
				smb.inCodeBlock = false
				return codeBlockContent, nil
			}

			smb.lastFlush = endPos
			smb.inCodeBlock = false
			return rendered, nil
		} else if endIdx := strings.Index(fullText[searchStart:], smb.codeBlockEnd); endIdx != -1 && searchStart+endIdx+len(smb.codeBlockEnd) == len(fullText) {
			// Found code block end at the very end of buffer (no trailing newline yet)
			endPos := searchStart + endIdx + len(smb.codeBlockEnd)
			codeBlockContent := fullText[smb.lastFlush:endPos]

			// Render the complete code block
			rendered, err := renderMarkdown(codeBlockContent)
			if err != nil {
				// Fallback to plain text
				smb.lastFlush = endPos
				smb.inCodeBlock = false
				return codeBlockContent, nil
			}

			smb.lastFlush = endPos
			smb.inCodeBlock = false
			return rendered, nil
		}

		// Still in code block, don't output anything until it's complete
		return "", nil
	}
}

// Flush renders and returns any remaining content in the buffer
func (smb *StreamingMarkdownBuffer) Flush(shouldUseMarkdown bool) (string, error) {
	fullText := smb.buffer.String()
	remainingContent := fullText[smb.lastFlush:]

	if remainingContent == "" {
		return "", nil
	}

	if !shouldUseMarkdown {
		return remainingContent, nil
	}

	rendered, err := renderMarkdown(remainingContent)
	if err != nil {
		return remainingContent, nil
	}

	return rendered, nil
}

// shouldUseMarkdown determines if Markdown rendering should be used based on color mode.
func shouldUseMarkdown(colorMode string) bool {
	supportsColor := func() bool {
		return !color.NoColor
	}

	switch colorMode {
	case "yes":
		return true
	case "no":
		return false
	case "auto":
		return supportsColor()
	default:
		return supportsColor()
	}
}

// getTerminalWidth returns the terminal width, with a fallback to 80.
func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 80
	}
	return width
}

// getMarkdownRenderer returns a Markdown renderer, recreating it if terminal width changed.
func getMarkdownRenderer() (*glamour.TermRenderer, error) {
	currentWidth := getTerminalWidth()

	// Recreate if width changed or renderer doesn't exist.
	if markdownRenderer == nil || currentWidth != lastWidth {
		r, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(currentWidth),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create markdown renderer: %w", err)
		}
		markdownRenderer = r
		lastWidth = currentWidth
	}

	return markdownRenderer, nil
}

func renderMarkdown(content string) (string, error) {
	r, err := getMarkdownRenderer()
	if err != nil {
		return "", fmt.Errorf("failed to create markdown renderer: %w", err)
	}

	rendered, err := r.Render(content)
	if err != nil {
		return "", fmt.Errorf("failed to render markdown: %w", err)
	}

	return rendered, nil
}

// buildUserMessage constructs an OpenAIChatMessage for the user with the processed prompt and images.
// This is used to ensure conversation history reflects exactly what the model received.
func buildUserMessage(prompt string, imageURLs []string) desktop.OpenAIChatMessage {
	if len(imageURLs) > 0 {
		// Multimodal message with images - build content array
		contentParts := make([]desktop.ContentPart, 0, len(imageURLs)+1)

		// Add all images first
		for _, imageURL := range imageURLs {
			contentParts = append(contentParts, desktop.ContentPart{
				Type: "image_url",
				ImageURL: &desktop.ImageURL{
					URL: imageURL,
				},
			})
		}

		// Add text prompt if present
		if prompt != "" {
			contentParts = append(contentParts, desktop.ContentPart{
				Type: "text",
				Text: prompt,
			})
		}

		return desktop.OpenAIChatMessage{
			Role:    "user",
			Content: contentParts,
		}
	}

	// Simple text-only message
	return desktop.OpenAIChatMessage{
		Role:    "user",
		Content: prompt,
	}
}

// chatWithMarkdown performs chat and streams the response with selective markdown rendering.
func chatWithMarkdown(cmd *cobra.Command, client *desktop.Client, model, prompt string) error {
	_, _, err := chatWithMarkdownContext(cmd.Context(), cmd, client, model, prompt, nil)
	return err
}

// chatWithMarkdownContext performs chat with context support and streams the response with selective markdown rendering.
// It accepts an optional conversation history and returns both the assistant's response and the processed user message
// (after file inclusions and image processing) for accurate history tracking.
func chatWithMarkdownContext(ctx context.Context, cmd *cobra.Command, client *desktop.Client, model, prompt string, conversationHistory []desktop.OpenAIChatMessage) (assistantResponse string, processedUserMessage desktop.OpenAIChatMessage, err error) {
	colorMode, _ := cmd.Flags().GetString("color")
	useMarkdown := shouldUseMarkdown(colorMode)
	debug, _ := cmd.Flags().GetBool("debug")

	// Process file inclusions first (files referenced with @ symbol)
	prompt, err = processFileInclusions(prompt)
	if err != nil {
		return "", desktop.OpenAIChatMessage{}, fmt.Errorf("failed to process file inclusions: %w", err)
	}

	var imageURLs []string
	cleanedPrompt, imgs, err := processImagesInPrompt(prompt)
	if err != nil {
		return "", desktop.OpenAIChatMessage{}, fmt.Errorf("failed to process images: %w", err)
	}
	prompt = cleanedPrompt
	imageURLs = imgs

	// Build the processed user message to return for history tracking.
	// This reflects exactly what the model receives.
	processedUserMessage = buildUserMessage(prompt, imageURLs)

	if !useMarkdown {
		// Simple case: just stream as plain text
		assistantResponse, err = client.ChatWithMessagesContext(ctx, model, conversationHistory, prompt, imageURLs, func(content string) {
			cmd.Print(content)
		}, false)
		return assistantResponse, processedUserMessage, err
	}

	// For markdown: use streaming buffer to render code blocks as they complete
	markdownBuffer := NewStreamingMarkdownBuffer()

	assistantResponse, err = client.ChatWithMessagesContext(ctx, model, conversationHistory, prompt, imageURLs, func(content string) {
		// Use the streaming markdown buffer to intelligently render content
		rendered, renderErr := markdownBuffer.AddContent(content, true)
		if renderErr != nil {
			if debug {
				cmd.PrintErrln(renderErr)
			}
			// Fallback to plain text on error
			cmd.Print(content)
		} else if rendered != "" {
			cmd.Print(rendered)
		}
	}, true)
	if err != nil {
		return assistantResponse, processedUserMessage, err
	}

	// Flush any remaining content from the markdown buffer
	if remaining, flushErr := markdownBuffer.Flush(true); flushErr == nil && remaining != "" {
		cmd.Print(remaining)
	}

	return assistantResponse, processedUserMessage, nil
}

func newRunCmd() *cobra.Command {
	var debug bool
	var colorMode string
	var detach bool
	var openaiURL string

	const cmdArgs = "MODEL [PROMPT]"
	c := &cobra.Command{
		Use:   "run " + cmdArgs,
		Short: "Run a model and interact with it using a submitted prompt or chat mode",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			switch colorMode {
			case "auto", "yes", "no":
				return nil
			default:
				return fmt.Errorf("--color must be one of: auto, yes, no (got %q)", colorMode)
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			model := args[0]
			prompt := ""
			argsLen := len(args)
			if argsLen > 1 {
				prompt = strings.Join(args[1:], " ")
			}

			// Only read from stdin if not in detach mode
			if !detach {
				fi, err := os.Stdin.Stat()
				if err == nil && (fi.Mode()&os.ModeCharDevice) == 0 {
					// Read all from stdin
					reader := bufio.NewReader(os.Stdin)
					input, err := io.ReadAll(reader)
					if err == nil {
						if prompt != "" {
							prompt += "\n\n"
						}

						prompt += string(input)
					}
				}
			}

			if debug {
				if prompt == "" {
					cmd.Printf("Running model %s\n", model)
				} else {
					cmd.Printf("Running model %s with prompt %s\n", model, prompt)
				}
			}

			// Handle --openaiurl flag for external OpenAI endpoints
			if openaiURL != "" {
				if detach {
					return fmt.Errorf("--detach flag cannot be used with --openaiurl flag")
				}
				ctx, err := desktop.NewContextForOpenAI(openaiURL)
				if err != nil {
					return fmt.Errorf("invalid OpenAI URL: %w", err)
				}
				openaiClient := desktop.New(ctx)

				if prompt != "" {
					// Single prompt mode
					useMarkdown := shouldUseMarkdown(colorMode)
					if err := openaiClient.ChatWithContext(cmd.Context(), model, prompt, nil, func(content string) {
						cmd.Print(content)
					}, useMarkdown); err != nil {
						return handleClientError(err, "Failed to generate a response")
					}
					cmd.Println()
					return nil
				}

				termenv.SetDefaultOutput(
					termenv.NewOutput(asPrinter(cmd), termenv.WithColorCache(true)),
				)
				return generateInteractiveWithReadline(cmd, openaiClient, model)
			}

			if _, err := ensureStandaloneRunnerAvailable(cmd.Context(), asPrinter(cmd), debug); err != nil {
				return fmt.Errorf("unable to initialize standalone model runner: %w", err)
			}

			// Check if this is an NVIDIA NIM image
			if isNIMImage(model) {
				// NIM images are handled differently - they run as Docker containers
				// Create a Docker client
				cli := getDockerCLI()
				dockerClient, err := desktop.DockerClientForContext(cli, cli.CurrentContext())
				if err != nil {
					return fmt.Errorf("failed to create Docker client: %w", err)
				}

				// Run the NIM model
				if err := runNIMModel(cmd.Context(), dockerClient, model, cmd); err != nil {
					return fmt.Errorf("failed to run NIM model: %w", err)
				}

				// If no prompt provided, enter interactive mode
				if prompt == "" {
					scanner := bufio.NewScanner(os.Stdin)
					cmd.Println("Interactive chat mode started. Type '/bye' to exit.")

					for {
						userInput, err := readMultilineInput(cmd, scanner)
						if err != nil {
							if err.Error() == "EOF" {
								cmd.Println("\nChat session ended.")
								break
							}
							return fmt.Errorf("Error reading input: %w", err)
						}

						if strings.ToLower(strings.TrimSpace(userInput)) == "/bye" {
							cmd.Println("Chat session ended.")
							break
						}

						if strings.TrimSpace(userInput) == "" {
							continue
						}

						if err := chatWithNIM(cmd, model, userInput); err != nil {
							cmd.PrintErr(fmt.Errorf("failed to chat with NIM: %w", err))
							continue
						}

						cmd.Println()
					}
					return nil
				}

				// Single prompt mode
				if err := chatWithNIM(cmd, model, prompt); err != nil {
					return fmt.Errorf("failed to chat with NIM: %w", err)
				}
				cmd.Println()
				return nil
			}

			_, err := desktopClient.Inspect(model, false)
			if err != nil {
				if !errors.Is(err, desktop.ErrNotFound) {
					return handleClientError(err, "Failed to inspect model")
				}
				cmd.Println("Unable to find model '" + model + "' locally. Pulling from the server.")
				if err := pullModel(cmd, desktopClient, model); err != nil {
					return err
				}
			}

			// Handle --detach flag: just load the model without interaction
			if detach {
				if err := desktopClient.Preload(cmd.Context(), model); err != nil {
					return handleClientError(err, "Failed to load model")
				}
				if debug {
					cmd.Printf("Model %s loaded successfully\n", model)
				}
				return nil
			}

			if prompt != "" {
				if err := chatWithMarkdown(cmd, desktopClient, model, prompt); err != nil {
					return handleClientError(err, "Failed to generate a response")
				}
				cmd.Println()
				return nil
			}

			// For interactive mode, eagerly load the model in the background
			// while the user types their first query
			go func() {
				if err := desktopClient.Preload(cmd.Context(), model); err != nil {
					cmd.PrintErrf("background model preload failed: %v\n", err)
				}
			}()

			// Initialize termenv with color caching before starting interactive session.
			// This queries the terminal background color once and caches it, preventing
			// OSC response sequences from appearing in stdin during the interactive loop.
			termenv.SetDefaultOutput(
				termenv.NewOutput(asPrinter(cmd), termenv.WithColorCache(true)),
			)

			return generateInteractiveWithReadline(cmd, desktopClient, model)

		},
		ValidArgsFunction: completion.ModelNames(getDesktopClient, 1),
	}
	c.Args = requireMinArgs(1, "run", cmdArgs)

	c.Flags().BoolVar(&debug, "debug", false, "Enable debug logging")
	c.Flags().StringVar(&colorMode, "color", "no", "Use colored output (auto|yes|no)")
	c.Flags().BoolVarP(&detach, "detach", "d", false, "Load the model in the background without interaction")
	c.Flags().StringVar(&openaiURL, "openaiurl", "", "OpenAI-compatible API endpoint URL to chat with")

	return c
}
