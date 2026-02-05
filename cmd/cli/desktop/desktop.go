package desktop

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/docker/model-runner/cmd/cli/pkg/standalone"
	"github.com/docker/model-runner/pkg/distribution/distribution"
	"github.com/docker/model-runner/pkg/inference"
	dmrm "github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/inference/scheduling"
	"github.com/fatih/color"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel"
)

var (
	ErrNotFound           = errors.New("model not found")
	ErrServiceUnavailable = errors.New("service unavailable")
)

type otelErrorSilencer struct{}

func (oes *otelErrorSilencer) Handle(error) {}

func init() {
	otel.SetErrorHandler(&otelErrorSilencer{})
}

type Client struct {
	modelRunner *ModelRunnerContext
}

//go:generate mockgen -source=desktop.go -destination=../mocks/mock_desktop.go -package=mocks DockerHttpClient
type DockerHttpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func New(modelRunner *ModelRunnerContext) *Client {
	return &Client{modelRunner}
}

type Status struct {
	Running bool   `json:"running"`
	Status  []byte `json:"status"`
	Error   error  `json:"error"`
}

func (c *Client) Status() Status {
	// TODO: Query "/".
	resp, err := c.doRequest(http.MethodGet, inference.ModelsPrefix, nil)
	if err != nil {
		err = c.handleQueryError(err, inference.ModelsPrefix)
		if errors.Is(err, ErrServiceUnavailable) {
			return Status{
				Running: false,
			}
		}
		return Status{
			Running: false,
			Error:   err,
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		var status []byte
		statusResp, err := c.doRequest(http.MethodGet, inference.InferencePrefix+"/status", nil)
		if err != nil {
			status = []byte(fmt.Sprintf("error querying status: %v", err))
		} else {
			defer statusResp.Body.Close()
			statusBody, err := io.ReadAll(statusResp.Body)
			if err != nil {
				status = []byte(fmt.Sprintf("error reading status body: %v", err))
			} else {
				status = statusBody
			}
		}
		return Status{
			Running: true,
			Status:  status,
		}
	}
	return Status{
		Running: false,
		Error:   fmt.Errorf("unexpected status code: %d", resp.StatusCode),
	}
}

func (c *Client) Pull(model string, printer standalone.StatusPrinter) (string, bool, error) {
	// Check if this is a Hugging Face model and if HF_TOKEN is set
	var hfToken string
	if strings.HasPrefix(strings.ToLower(model), "hf.co/") {
		hfToken = os.Getenv("HF_TOKEN")
	}

	return c.withRetries("download", 3, printer, func(attempt int) (string, bool, error, bool) {
		jsonData, err := json.Marshal(dmrm.ModelCreateRequest{
			From:        model,
			BearerToken: hfToken,
		})
		if err != nil {
			// Marshaling errors are not retryable
			return "", false, fmt.Errorf("error marshaling request: %w", err), false
		}

		createPath := inference.ModelsPrefix + "/create"
		resp, err := c.doRequest(
			http.MethodPost,
			createPath,
			bytes.NewReader(jsonData),
		)
		if err != nil {
			// Only retry on network errors, not on client errors
			if isRetryableError(err) {
				return "", false, c.handleQueryError(err, createPath), true
			}
			return "", false, c.handleQueryError(err, createPath), false
		}
		// Close response body explicitly at the end of this attempt, not deferred
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			err := fmt.Errorf("pulling %s failed with status %s: %s", model, resp.Status, string(body))
			// Only retry on server errors (5xx), not client errors (4xx)
			shouldRetry := resp.StatusCode >= 500 && resp.StatusCode < 600
			return "", false, err, shouldRetry
		}

		// Use Docker-style progress display
		message, shown, err := DisplayProgress(resp.Body, printer)
		if err != nil {
			// Retry on progress display errors (likely network interruption)
			shouldRetry := isRetryableError(err)
			return "", shown, err, shouldRetry
		}

		return message, shown, nil, false
	})
}

// isRetryableError determines if an error is retryable (network-related)
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// First check for specific error types using errors.Is
	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, ErrServiceUnavailable) {
		return true
	}

	// Fall back to string matching for network errors that don't have specific types
	// This is necessary because many network errors are only available as strings
	errStr := err.Error()
	retryablePatterns := []string{
		"connection refused",
		"connection reset",
		"broken pipe",
		"timeout",
		"temporary failure",
		"no such host",
		"no route to host",
		"network is unreachable",
		"i/o timeout",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(strings.ToLower(errStr), pattern) {
			return true
		}
	}

	return false
}

// withRetries executes an operation with automatic retry logic for transient failures
func (c *Client) withRetries(
	operationName string,
	maxRetries int,
	printer standalone.StatusPrinter,
	operation func(attempt int) (message string, progressShown bool, err error, shouldRetry bool),
) (string, bool, error) {
	var lastErr error
	var progressShown bool

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate exponential backoff: 2^(attempt-1) seconds (1s, 2s, 4s)
			backoffDuration := time.Duration(1<<uint(attempt-1)) * time.Second
			printer.PrintErrf("Retrying %s (attempt %d/%d) in %v...\n", operationName, attempt, maxRetries, backoffDuration)
			time.Sleep(backoffDuration)
		}

		message, shown, err, shouldRetry := operation(attempt)
		progressShown = progressShown || shown

		if err == nil {
			return message, progressShown, nil
		}

		lastErr = err
		if !shouldRetry {
			return "", progressShown, err
		}
	}

	return "", progressShown, fmt.Errorf("failed to %s after %d retries: %w", operationName, maxRetries, lastErr)
}

func (c *Client) Push(model string, printer standalone.StatusPrinter) (string, bool, error) {
	return c.withRetries("push", 3, printer, func(attempt int) (string, bool, error, bool) {
		pushPath := inference.ModelsPrefix + "/" + model + "/push"
		resp, err := c.doRequest(
			http.MethodPost,
			pushPath,
			nil, // Assuming no body is needed for the push request
		)
		if err != nil {
			// Only retry on network errors, not on client errors
			if isRetryableError(err) {
				return "", false, c.handleQueryError(err, pushPath), true
			}
			return "", false, c.handleQueryError(err, pushPath), false
		}
		// Close response body explicitly at the end of this attempt, not deferred
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			err := fmt.Errorf("pushing %s failed with status %s: %s", model, resp.Status, string(body))
			// Only retry on server errors (5xx), not client errors (4xx)
			shouldRetry := resp.StatusCode >= 500 && resp.StatusCode < 600
			return "", false, err, shouldRetry
		}

		// Use Docker-style progress display
		message, shown, err := DisplayProgress(resp.Body, printer)
		if err != nil {
			// Retry on progress display errors (likely network interruption)
			shouldRetry := isRetryableError(err)
			return "", shown, err, shouldRetry
		}

		return message, shown, nil, false
	})
}

func (c *Client) List() ([]dmrm.Model, error) {
	modelsRoute := inference.ModelsPrefix
	body, err := c.listRaw(modelsRoute, "")
	if err != nil {
		return []dmrm.Model{}, err
	}

	var modelsJson []dmrm.Model
	if err := json.Unmarshal(body, &modelsJson); err != nil {
		return modelsJson, fmt.Errorf("failed to unmarshal response body: %w", err)
	}

	return modelsJson, nil
}

func (c *Client) ListOpenAI() (dmrm.OpenAIModelList, error) {
	modelsRoute := c.modelRunner.OpenAIPathPrefix() + "/models"
	body, err := c.listRaw(modelsRoute, "")
	if err != nil {
		return dmrm.OpenAIModelList{}, err
	}

	var modelsJson dmrm.OpenAIModelList
	if err := json.Unmarshal(body, &modelsJson); err != nil {
		return modelsJson, fmt.Errorf("failed to unmarshal response body: %w", err)
	}

	return modelsJson, nil
}

func (c *Client) Inspect(model string, remote bool) (dmrm.Model, error) {
	rawResponse, err := c.listRawWithQuery(fmt.Sprintf("%s/%s", inference.ModelsPrefix, model), model, remote)
	if err != nil {
		return dmrm.Model{}, err
	}
	var modelInspect dmrm.Model
	if err := json.Unmarshal(rawResponse, &modelInspect); err != nil {
		return modelInspect, fmt.Errorf("failed to unmarshal response body: %w", err)
	}

	return modelInspect, nil
}

func (c *Client) InspectOpenAI(model string) (dmrm.OpenAIModel, error) {
	modelsRoute := c.modelRunner.OpenAIPathPrefix() + "/models"
	rawResponse, err := c.listRaw(fmt.Sprintf("%s/%s", modelsRoute, model), model)
	if err != nil {
		return dmrm.OpenAIModel{}, err
	}
	var modelInspect dmrm.OpenAIModel
	if err := json.Unmarshal(rawResponse, &modelInspect); err != nil {
		return modelInspect, fmt.Errorf("failed to unmarshal response body: %w", err)
	}
	return modelInspect, nil
}

func (c *Client) listRaw(route string, model string) ([]byte, error) {
	return c.listRawWithQuery(route, model, false)
}

func (c *Client) listRawWithQuery(route string, model string, remote bool) ([]byte, error) {
	if remote {
		route += "?remote=true"
	}

	resp, err := c.doRequest(http.MethodGet, route, nil)
	if err != nil {
		return nil, c.handleQueryError(err, route)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if model != "" && resp.StatusCode == http.StatusNotFound {
			return nil, errors.Wrap(ErrNotFound, model)
		}
		return nil, fmt.Errorf("failed to list models: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	return body, nil
}

// Chat performs a chat request and streams the response content with selective markdown rendering.
func (c *Client) Chat(model, prompt string, imageURLs []string, outputFunc func(string), shouldUseMarkdown bool) error {
	return c.ChatWithContext(context.Background(), model, prompt, imageURLs, outputFunc, shouldUseMarkdown)
}

// Preload loads a model into memory without running inference.
// The model stays loaded for the idle timeout period.
func (c *Client) Preload(ctx context.Context, model string) error {
	reqBody := OpenAIChatRequest{
		Model:    model,
		Messages: []OpenAIChatMessage{},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("error marshaling request: %w", err)
	}

	completionsPath := c.modelRunner.OpenAIPathPrefix() + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.modelRunner.URL(completionsPath), bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "docker-model-cli/"+Version)
	req.Header.Set("X-Preload-Only", "true")

	resp, err := c.modelRunner.Client().Do(req)
	if err != nil {
		return c.handleQueryError(err, completionsPath)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("preload failed with status %d and could not read response body: %w", resp.StatusCode, err)
		}
		return fmt.Errorf("preload failed: status=%d body=%s", resp.StatusCode, body)
	}

	return nil
}

// ChatWithMessagesContext performs a chat request with conversation history and returns the assistant's response.
// This allows maintaining conversation context across multiple exchanges.
func (c *Client) ChatWithMessagesContext(ctx context.Context, model string, conversationHistory []OpenAIChatMessage, prompt string, imageURLs []string, outputFunc func(string), shouldUseMarkdown bool) (string, error) {
	// Build the current user message content - either simple string or multimodal array
	var messageContent interface{}
	if len(imageURLs) > 0 {
		// Multimodal message with images
		contentParts := make([]ContentPart, 0, len(imageURLs)+1)

		// Add all images first
		for _, imageURL := range imageURLs {
			contentParts = append(contentParts, ContentPart{
				Type: "image_url",
				ImageURL: &ImageURL{
					URL: imageURL,
				},
			})
		}

		// Add text prompt if present
		if prompt != "" {
			contentParts = append(contentParts, ContentPart{
				Type: "text",
				Text: prompt,
			})
		}

		messageContent = contentParts
	} else {
		// Simple text-only message
		messageContent = prompt
	}

	// Build messages array with conversation history plus current message
	messages := make([]OpenAIChatMessage, 0, len(conversationHistory)+1)
	messages = append(messages, conversationHistory...)
	messages = append(messages, OpenAIChatMessage{
		Role:    "user",
		Content: messageContent,
	})

	reqBody := OpenAIChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   true,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("error marshaling request: %w", err)
	}

	completionsPath := c.modelRunner.OpenAIPathPrefix() + "/chat/completions"

	resp, err := c.doRequestWithAuthContext(
		ctx,
		http.MethodPost,
		completionsPath,
		bytes.NewReader(jsonData),
	)
	if err != nil {
		return "", c.handleQueryError(err, completionsPath)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("error response: status=%d body=%s", resp.StatusCode, body)
	}

	type chatPrinterState int
	const (
		chatPrinterNone chatPrinterState = iota
		chatPrinterReasoning
		chatPrinterContent
	)

	printerState := chatPrinterNone
	reasoningFmt := color.New().Add(color.Italic)
	if !shouldUseMarkdown {
		reasoningFmt.DisableColor()
	}

	var assistantResponse strings.Builder
	var finalUsage *struct {
		CompletionTokens int `json:"completion_tokens"`
		PromptTokens     int `json:"prompt_tokens"`
		TotalTokens      int `json:"total_tokens"`
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		// Check if context was cancelled
		select {
		case <-ctx.Done():
			return assistantResponse.String(), ctx.Err()
		default:
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		if data == "[DONE]" {
			break
		}

		var streamResp OpenAIChatResponse
		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			return assistantResponse.String(), fmt.Errorf("error parsing stream response: %w", err)
		}

		if streamResp.Usage != nil {
			finalUsage = streamResp.Usage
		}

		if len(streamResp.Choices) > 0 {
			if streamResp.Choices[0].Delta.ReasoningContent != "" {
				chunk := streamResp.Choices[0].Delta.ReasoningContent
				if printerState == chatPrinterContent {
					outputFunc("\n\n")
				}
				if printerState != chatPrinterReasoning {
					const thinkingHeader = "Thinking:\n"
					if reasoningFmt != nil {
						reasoningFmt.Print(thinkingHeader)
					} else {
						outputFunc(thinkingHeader)
					}
				}
				printerState = chatPrinterReasoning
				if reasoningFmt != nil {
					reasoningFmt.Print(chunk)
				} else {
					outputFunc(chunk)
				}
			}
			if streamResp.Choices[0].Delta.Content != "" {
				chunk := streamResp.Choices[0].Delta.Content
				if printerState == chatPrinterReasoning {
					outputFunc("\n\n--\n\n")
				}
				printerState = chatPrinterContent
				outputFunc(chunk)
				assistantResponse.WriteString(chunk)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return assistantResponse.String(), fmt.Errorf("error reading response stream: %w", err)
	}

	if finalUsage != nil {
		usageInfo := fmt.Sprintf("\n\nToken usage: %d prompt + %d completion = %d total",
			finalUsage.PromptTokens,
			finalUsage.CompletionTokens,
			finalUsage.TotalTokens)

		usageFmt := color.New(color.FgHiBlack)
		if !shouldUseMarkdown {
			usageFmt.DisableColor()
		}
		outputFunc(usageFmt.Sprint(usageInfo))
	}

	return assistantResponse.String(), nil
}

// ChatWithContext performs a chat request with context support for cancellation and streams the response content with selective markdown rendering.
func (c *Client) ChatWithContext(ctx context.Context, model, prompt string, imageURLs []string, outputFunc func(string), shouldUseMarkdown bool) error {
	_, err := c.ChatWithMessagesContext(ctx, model, nil, prompt, imageURLs, outputFunc, shouldUseMarkdown)
	return err
}

func (c *Client) Remove(modelArgs []string, force bool) (string, error) {
	modelRemoved := ""
	for _, model := range modelArgs {
		// Construct the URL with query parameters
		removePath := fmt.Sprintf("%s/%s?force=%s",
			inference.ModelsPrefix,
			model,
			strconv.FormatBool(force),
		)

		resp, err := c.doRequest(http.MethodDelete, removePath, nil)
		if err != nil {
			return modelRemoved, c.handleQueryError(err, removePath)
		}
		defer resp.Body.Close()

		var bodyStr string
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			bodyStr = fmt.Sprintf("(failed to read response body: %v)", err)
		} else {
			bodyStr = string(body)
		}

		if resp.StatusCode == http.StatusOK {
			var deleteResponse distribution.DeleteModelResponse
			if err := json.Unmarshal(body, &deleteResponse); err != nil {
				modelRemoved += fmt.Sprintf("Model %s removed successfully, but failed to parse response: %v\n", model, err)
			} else {
				for _, msg := range deleteResponse {
					if msg.Untagged != nil {
						modelRemoved += fmt.Sprintf("Untagged: %s\n", *msg.Untagged)
					}
					if msg.Deleted != nil {
						modelRemoved += fmt.Sprintf("Deleted: %s\n", *msg.Deleted)
					}
				}
			}
		} else {
			if resp.StatusCode == http.StatusNotFound {
				return modelRemoved, fmt.Errorf("no such model: %s", model)
			}
			return modelRemoved, fmt.Errorf("removing %s failed with status %s: %s", model, resp.Status, bodyStr)
		}
	}
	return modelRemoved, nil
}

// BackendStatus to be imported from docker/model-runner when https://github.com/docker/model-runner/pull/42 is merged.
type BackendStatus struct {
	// BackendName is the name of the backend
	BackendName string `json:"backend_name"`
	// ModelName is the name of the model loaded in the backend
	ModelName string `json:"model_name"`
	// Mode is the mode the backend is operating in
	Mode string `json:"mode"`
	// LastUsed represents when this backend was last used (if it's idle)
	LastUsed time.Time `json:"last_used,omitempty"`
	// InUse indicates whether this backend is currently handling a request
	InUse bool `json:"in_use,omitempty"`
}

func (c *Client) PS() ([]BackendStatus, error) {
	psPath := inference.InferencePrefix + "/ps"
	resp, err := c.doRequest(http.MethodGet, psPath, nil)
	if err != nil {
		return []BackendStatus{}, c.handleQueryError(err, psPath)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return []BackendStatus{}, fmt.Errorf("failed to list running models: %s", resp.Status)
	}

	body, _ := io.ReadAll(resp.Body)
	var ps []BackendStatus
	if err := json.Unmarshal(body, &ps); err != nil {
		return []BackendStatus{}, fmt.Errorf("failed to unmarshal response body: %w", err)
	}

	return ps, nil
}

// DiskUsage to be imported from docker/model-runner when https://github.com/docker/model-runner/pull/45 is merged.
type DiskUsage struct {
	ModelsDiskUsage         int64 `json:"models_disk_usage"`
	DefaultBackendDiskUsage int64 `json:"default_backend_disk_usage"`
}

func (c *Client) DF() (DiskUsage, error) {
	dfPath := inference.InferencePrefix + "/df"
	resp, err := c.doRequest(http.MethodGet, dfPath, nil)
	if err != nil {
		return DiskUsage{}, c.handleQueryError(err, dfPath)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return DiskUsage{}, fmt.Errorf("failed to get disk usage: %s", resp.Status)
	}

	body, _ := io.ReadAll(resp.Body)
	var df DiskUsage
	if err := json.Unmarshal(body, &df); err != nil {
		return DiskUsage{}, fmt.Errorf("failed to unmarshal response body: %w", err)
	}

	return df, nil
}

// UnloadRequest to be imported from docker/model-runner when https://github.com/docker/model-runner/pull/46 is merged.
type UnloadRequest struct {
	All     bool     `json:"all"`
	Backend string   `json:"backend"`
	Models  []string `json:"models"`
}

// UnloadResponse to be imported from docker/model-runner when https://github.com/docker/model-runner/pull/46 is merged.
type UnloadResponse struct {
	UnloadedRunners int `json:"unloaded_runners"`
}

func (c *Client) Unload(req UnloadRequest) (UnloadResponse, error) {
	unloadPath := inference.InferencePrefix + "/unload"
	jsonData, err := json.Marshal(req)
	if err != nil {
		return UnloadResponse{}, fmt.Errorf("error marshaling request: %w", err)
	}

	resp, err := c.doRequest(http.MethodPost, unloadPath, bytes.NewReader(jsonData))
	if err != nil {
		return UnloadResponse{}, c.handleQueryError(err, unloadPath)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return UnloadResponse{}, fmt.Errorf("unloading failed with status %s: %s", resp.Status, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return UnloadResponse{}, fmt.Errorf("failed to read response body: %w", err)
	}

	var unloadResp UnloadResponse
	if err := json.Unmarshal(body, &unloadResp); err != nil {
		return UnloadResponse{}, fmt.Errorf("failed to unmarshal response body: %w", err)
	}

	return unloadResp, nil
}

func (c *Client) ShowConfigs(modelFilter string) ([]scheduling.ModelConfigEntry, error) {
	configureBackendPath := inference.InferencePrefix + "/_configure"
	if modelFilter != "" {
		configureBackendPath += "?model=" + url.QueryEscape(modelFilter)
	}
	resp, err := c.doRequest(http.MethodGet, configureBackendPath, nil)
	if err != nil {
		return nil, c.handleQueryError(err, configureBackendPath)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("listing configs failed with status %s: %s", resp.Status, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var configs []scheduling.ModelConfigEntry
	if err := json.Unmarshal(body, &configs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response body: %w", err)
	}

	return configs, nil
}

func (c *Client) ConfigureBackend(request scheduling.ConfigureRequest) error {
	configureBackendPath := inference.InferencePrefix + "/_configure"
	jsonData, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("error marshaling request: %w", err)
	}

	resp, err := c.doRequest(http.MethodPost, configureBackendPath, bytes.NewReader(jsonData))
	if err != nil {
		return c.handleQueryError(err, configureBackendPath)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusConflict {
			return fmt.Errorf("%s", body)
		}
		return fmt.Errorf("%s (%s)", body, resp.Status)
	}

	return nil
}

// Requests returns a response body and a cancel function to ensure proper cleanup.
func (c *Client) Requests(modelFilter string, streaming bool, includeExisting bool) (io.ReadCloser, func(), error) {
	path := c.modelRunner.URL(inference.InferencePrefix + "/requests")
	var queryParams []string
	if modelFilter != "" {
		queryParams = append(queryParams, "model="+url.QueryEscape(modelFilter))
	}
	if includeExisting && streaming {
		queryParams = append(queryParams, "include_existing=true")
	}
	if len(queryParams) > 0 {
		path += "?" + strings.Join(queryParams, "&")
	}

	req, err := http.NewRequest(http.MethodGet, path, http.NoBody)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	if streaming {
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Cache-Control", "no-cache")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	req.Header.Set("User-Agent", "docker-model-cli/"+Version)

	resp, err := c.modelRunner.Client().Do(req)
	if err != nil {
		if streaming {
			return nil, nil, c.handleQueryError(fmt.Errorf("failed to connect to stream: %w", err), path)
		}
		return nil, nil, c.handleQueryError(err, path)
	}

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, nil, fmt.Errorf("%s", strings.TrimSpace(string(body)))
		}

		resp.Body.Close()
		if streaming {
			return nil, nil, fmt.Errorf("stream request failed with status: %d", resp.StatusCode)
		}
		return nil, nil, fmt.Errorf("failed to list requests: %s", resp.Status)
	}

	// Return the response body and a cancel function that closes it.
	cancel := func() {
		resp.Body.Close()
	}

	return resp.Body, cancel, nil
}

func (c *Client) Purge() error {
	purgePath := inference.ModelsPrefix + "/purge"
	resp, err := c.doRequest(http.MethodDelete, purgePath, nil)
	if err != nil {
		return c.handleQueryError(err, purgePath)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("purging failed with status %s: %s", resp.Status, string(body))
	}

	return nil
}

// doRequest is a helper function that performs HTTP requests and handles 503 responses
func (c *Client) doRequest(method, path string, body io.Reader) (*http.Response, error) {
	return c.doRequestWithAuth(method, path, body)
}

// doRequestWithAuth is a helper function that performs HTTP requests with optional authentication
func (c *Client) doRequestWithAuth(method, path string, body io.Reader) (*http.Response, error) {
	return c.doRequestWithAuthContext(context.Background(), method, path, body)
}

func (c *Client) doRequestWithAuthContext(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.modelRunner.URL(path), body)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	req.Header.Set("User-Agent", "docker-model-cli/"+Version)

	resp, err := c.modelRunner.Client().Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusServiceUnavailable {
		resp.Body.Close()
		return nil, ErrServiceUnavailable
	}

	return resp, nil
}

func (c *Client) handleQueryError(err error, path string) error {
	if errors.Is(err, ErrServiceUnavailable) {
		return ErrServiceUnavailable
	}
	return fmt.Errorf("error querying %s: %w", path, err)
}

func (c *Client) Tag(source, targetRepo, targetTag string) error {
	// Construct the URL with query parameters using the normalized source
	tagPath := fmt.Sprintf("%s/%s/tag?repo=%s&tag=%s",
		inference.ModelsPrefix,
		source,
		targetRepo,
		targetTag,
	)

	resp, err := c.doRequest(http.MethodPost, tagPath, nil)
	if err != nil {
		return c.handleQueryError(err, tagPath)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("tagging failed with status %s: %s", resp.Status, string(body))
	}

	return nil
}

func (c *Client) LoadModel(ctx context.Context, r io.Reader) error {
	loadPath := fmt.Sprintf("%s/load", inference.ModelsPrefix)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.modelRunner.URL(loadPath), r)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-tar")
	req.Header.Set("User-Agent", "docker-model-cli/"+Version)

	resp, err := c.modelRunner.Client().Do(req)
	if err != nil {
		return c.handleQueryError(err, loadPath)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("load failed with status %s: %s", resp.Status, string(body))
	}
	return nil
}

func (c *Client) ExportModel(ctx context.Context, model string) (io.ReadCloser, error) {
	exportPath := fmt.Sprintf("%s/%s/export", inference.ModelsPrefix, model)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.modelRunner.URL(exportPath), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "docker-model-cli/"+Version)

	resp, err := c.modelRunner.Client().Do(req)
	if err != nil {
		return nil, c.handleQueryError(err, exportPath)
	}

	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, errors.Wrap(ErrNotFound, model)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("export failed with status %s: %s", resp.Status, string(body))
	}

	return resp.Body, nil
}

type RepackageOptions struct {
	ContextSize *uint64 `json:"context_size,omitempty"`
}

func (c *Client) RepackageModel(ctx context.Context, source, target string, opts RepackageOptions) error {
	repackagePath := fmt.Sprintf("%s/%s/repackage", inference.ModelsPrefix, source)

	reqBody := struct {
		Target      string  `json:"target"`
		ContextSize *uint64 `json:"context_size,omitempty"`
	}{
		Target:      target,
		ContextSize: opts.ContextSize,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("error marshaling request: %w", err)
	}

	resp, err := c.doRequestWithAuthContext(ctx, http.MethodPost, repackagePath, bytes.NewReader(jsonData))
	if err != nil {
		return c.handleQueryError(err, repackagePath)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return errors.Wrap(ErrNotFound, source)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("repackage failed with status %s: %s", resp.Status, string(body))
	}

	return nil
}
