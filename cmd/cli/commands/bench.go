package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/docker/model-runner/cmd/cli/desktop"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/spf13/cobra"
)

var (
	defaultPrompt = "Write a comprehensive 100 word summary on whales and their impact on society."
)

type BenchmarkResult struct {
	Concurrency int
	MeanRPS     float64
	TotalTokens int
	TPS         float64
	TotalTime   time.Duration
	Requests    int
	TokenCounts []int
}

type ChatResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func newBenchCmd() *cobra.Command {
	var (
		prompt     string
		duration   time.Duration
		model      string
		jsonOutput bool
		numWorkers []int
		timeout    time.Duration
	)

	cmd := &cobra.Command{
		Use:   "bench [MODEL]",
		Short: "Benchmark a model's performance at different concurrency levels",
		Long: `Benchmark a model's performance showing tokens per second at different concurrency levels.

This command runs a series of benchmarks with 1, 2, 4, and 8 concurrent requests by default,
measuring the tokens per second (TPS) that the model can generate.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			model = args[0]

			if duration == 0 {
				duration = 30 * time.Second // Default to 30 seconds per concurrency test
			}
			if timeout == 0 {
				timeout = 5 * time.Minute // Default timeout for each request
			}
			if len(numWorkers) == 0 {
				numWorkers = []int{1, 2, 4, 8} // Default concurrency levels
			}

			// Validate model exists
			_, err := desktopClient.Inspect(model, false)
			if err != nil {
				return handleClientError(err, "Failed to inspect model")
			}

			// Ensure model runner is available
			if _, err := ensureStandaloneRunnerAvailable(cmd.Context(), asPrinter(cmd), false); err != nil {
				return fmt.Errorf("unable to initialize standalone model runner: %w", err)
			}

			if !jsonOutput {
				fmt.Printf("Prompt: %s\n", prompt)
				fmt.Printf("Duration: %v per concurrency level\n", duration)
				fmt.Printf("Concurrency levels: %v\n", numWorkers)
				fmt.Println()
			}

			results := make([]BenchmarkResult, 0, len(numWorkers))

			for _, workers := range numWorkers {
				if !jsonOutput {
					fmt.Printf("Running benchmark with concurrency: %d\n", workers)
				}

				result, err := runBenchmark(cmd.Context(), model, prompt, workers, duration, timeout)
				if err != nil {
					return fmt.Errorf("benchmark failed for concurrency %d: %w", workers, err)
				}

				results = append(results, result)

				if !jsonOutput {
					fmt.Printf("  Tokens per second: %.2f TPS\n", result.TPS)
					fmt.Printf("  Requests per second: %.2f RPS\n", result.MeanRPS)
					fmt.Printf("  Total tokens: %d\n", result.TotalTokens)
					fmt.Printf("  Total requests: %d\n", result.Requests)
					fmt.Printf("  Total time: %v\n", result.TotalTime)
					fmt.Println()
				}
			}

			// Sort results by concurrency for consistent output
			sort.Slice(results, func(i, j int) bool {
				return results[i].Concurrency < results[j].Concurrency
			})

			if jsonOutput {
				output := map[string]interface{}{
					"model":   model,
					"prompt":  prompt,
					"results": results,
				}
				jsonBytes, err := json.MarshalIndent(output, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to marshal JSON output: %w", err)
				}
				fmt.Println(string(jsonBytes))
			} else {
				printBenchmarkTable(results)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&prompt, "prompt", defaultPrompt, "Prompt to use for benchmarking")
	cmd.Flags().DurationVar(&duration, "duration", 30*time.Second, "Duration to run each concurrency test")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output results in JSON format")
	cmd.Flags().IntSliceVar(&numWorkers, "concurrency", []int{1, 2, 4, 8}, "Concurrency levels to test")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "Timeout for each individual request")

	return cmd
}

func runBenchmark(ctx context.Context, model, prompt string, numWorkers int, duration time.Duration, timeout time.Duration) (BenchmarkResult, error) {
	// Create channels for request/response
	requests := make(chan struct{}, numWorkers*2)
	results := make(chan int, numWorkers*2)

	// Start worker goroutines
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range requests {
				// Make request to the model with timeout
				reqCtx, cancel := context.WithTimeout(ctx, timeout)
				tokens, err := sendChatRequest(reqCtx, model, prompt)
				cancel()
				if err != nil {
					// Log error but continue
					fmt.Fprintf(os.Stderr, "request failed during benchmark: %v\n", err)
					continue
				}
				// Try to send results, but don't block if results channel is full or closed
				select {
				case results <- tokens:
				default:
					// If results channel is full, just continue to avoid blocking
				}
			}
		}()
	}

	// Start timer
	startTime := time.Now()
	endTime := startTime.Add(duration)

	// Send requests at a steady rate
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond) // Send ~100 requests per second
		defer ticker.Stop()
		defer close(requests)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if time.Now().After(endTime) {
					return
				}
				select {
				case requests <- struct{}{}:
				default:
					// Channel is full, skip
				}
			}
		}
	}()

	// Collect results until timer expires
	tokenCounts := []int{}
	var totalTokens int
	requestCount := 0

	// Use a separate goroutine to close results channel after workers finish
	go func() {
		wg.Wait()      // Wait for all workers to finish
		close(results) // Now it's safe to close the results channel
	}()

	// Collect results until results channel is closed
	for tokens := range results {
		tokenCounts = append(tokenCounts, tokens)
		totalTokens += tokens
		requestCount++
	}

	totalTime := time.Since(startTime)

	// Calculate statistics
	rps := float64(requestCount) / totalTime.Seconds()
	tps := float64(totalTokens) / totalTime.Seconds()

	return BenchmarkResult{
		Concurrency: numWorkers,
		MeanRPS:     rps,
		TotalTokens: totalTokens,
		TPS:         tps,
		TotalTime:   totalTime,
		Requests:    requestCount,
		TokenCounts: tokenCounts,
	}, nil
}

func sendChatRequest(ctx context.Context, model, prompt string) (int, error) {
	// Use the model runner's client to make a request to the inference endpoint
	reqBody := desktop.OpenAIChatRequest{
		Model: model,
		Messages: []desktop.OpenAIChatMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Stream: false, // Non-streaming to get complete response with token counts
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return 0, fmt.Errorf("error marshaling request: %w", err)
	}

	// Create HTTP request using the model runner's URL method
	url := modelRunner.URL(inference.InferencePrefix + "/v1/chat/completions")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return 0, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "docker-model-cli/"+desktop.Version)

	// Execute request using the model runner's client
	resp, err := modelRunner.Client().Do(req)
	if err != nil {
		return 0, fmt.Errorf("error executing request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("error reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(responseBody))
	}

	// Parse response to get token usage
	var chatResp ChatResponse
	if err := json.Unmarshal(responseBody, &chatResp); err != nil {
		return 0, fmt.Errorf("error parsing response: %w", err)
	}

	// Check if we have usage information
	if chatResp.Usage.CompletionTokens > 0 {
		return chatResp.Usage.CompletionTokens, nil
	}

	// Fallback: estimate based on content if no usage info available
	// This is a rough estimation and should be improved
	content := ""
	if len(chatResp.Choices) > 0 {
		content = chatResp.Choices[0].Message.Content
	}
	estimatedTokens := len(content) / 4 // Rough estimate: 1 token ~ 4 characters
	return estimatedTokens, nil
}

func printBenchmarkTable(results []BenchmarkResult) {
	if len(results) == 0 {
		return
	}

	// Sort results by concurrency for consistent display
	sort.Slice(results, func(i, j int) bool {
		return results[i].Concurrency < results[j].Concurrency
	})

	// Calculate relative performance - use 1st concurrency as baseline if available
	var baseTPS float64
	if len(results) > 0 {
		// Find the result with concurrency 1 as baseline, if available
		for _, r := range results {
			if r.Concurrency == 1 {
				baseTPS = r.TPS
				break
			}
		}
		// If no concurrency 1, use the first result as baseline
		if baseTPS == 0 && len(results) > 0 {
			baseTPS = results[0].TPS
		}
	}

	// Print results in a hyperfine-like format
	fmt.Println()
	fmt.Println("Benchmark #1: Tokens per Second at Different Concurrency Levels")
	fmt.Println()

	// Create table with hyperfine-style formatting
	maxTPS := 0.0
	for _, r := range results {
		if r.TPS > maxTPS {
			maxTPS = r.TPS
		}
	}

	for _, r := range results {
		relSpeed := 1.0
		if baseTPS > 0 {
			relSpeed = r.TPS / baseTPS
		}

		status := ""
		if r.TPS == maxTPS && maxTPS > 0 {
			status = " fastest"
		}

		fmt.Printf("Concurrency %d: %.2f tokens/sec (rel. speed: %.2fx)%s\n",
			r.Concurrency, r.TPS, relSpeed, status)
	}

	fmt.Println()

	// Summary table
	fmt.Println("Summary:")
	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("%-25s %-20s\n", "Metric", "Value")
	fmt.Println(strings.Repeat("-", 50))

	if maxTPS > 0 {
		bestConcurrency := 0
		for _, r := range results {
			if r.TPS == maxTPS {
				bestConcurrency = r.Concurrency
				break
			}
		}

		fmt.Printf("%-25s %-20.2f\n", "Max Tokens/sec", maxTPS)
		fmt.Printf("%-25s %-20d\n", "Best concurrency", bestConcurrency)

		// Calculate improvement from 1 to best concurrency
		if bestConcurrency > 1 && baseTPS > 0 {
			improvement := ((maxTPS - baseTPS) / baseTPS) * 100
			fmt.Printf("%-25s %-20.1f%%\n", "Improvement from 1", improvement)
		}
	}

	fmt.Println(strings.Repeat("-", 50))
	fmt.Println()

	// Detailed performance table
	fmt.Println("Detailed Results:")
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("%-12s %-15s %-15s %-15s %-15s\n",
		"Concurrency", "Tokens/sec", "Rel. Speed", "Requests/sec", "Total Tokens")
	fmt.Println(strings.Repeat("-", 80))

	for _, r := range results {
		relSpeed := 1.0
		if baseTPS > 0 {
			relSpeed = r.TPS / baseTPS
		}

		fmt.Printf("%-12d %-15.2f %-15.2f %-15.2f %-15d\n",
			r.Concurrency,
			r.TPS,
			relSpeed,
			r.MeanRPS,
			r.TotalTokens)
	}
	fmt.Println(strings.Repeat("-", 80))
}

// Add the command to the root command
func init() {
	// We don't add it here since it's added in root.go
}
