package desktop

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/docker/go-units"
	"github.com/docker/model-runner/pkg/distribution/distribution"
	"github.com/docker/model-runner/pkg/inference"
	dmrm "github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/inference/scheduling"
	"github.com/fatih/color"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel"
)

const DefaultBackend = "llama.cpp"

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

// normalizeHuggingFaceModelName converts Hugging Face model names to lowercase
func normalizeHuggingFaceModelName(model string) string {
	if strings.HasPrefix(model, "hf.co/") {
		return strings.ToLower(model)
	}
	return model
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

func (c *Client) Pull(model string, ignoreRuntimeMemoryCheck bool, progress func(string)) (string, bool, error) {
	model = normalizeHuggingFaceModelName(model)
	jsonData, err := json.Marshal(dmrm.ModelCreateRequest{From: model, IgnoreRuntimeMemoryCheck: ignoreRuntimeMemoryCheck})
	if err != nil {
		return "", false, fmt.Errorf("error marshaling request: %w", err)
	}

	createPath := inference.ModelsPrefix + "/create"
	resp, err := c.doRequest(
		http.MethodPost,
		createPath,
		bytes.NewReader(jsonData),
	)
	if err != nil {
		return "", false, c.handleQueryError(err, createPath)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", false, fmt.Errorf("pulling %s failed with status %s: %s", model, resp.Status, string(body))
	}

	progressShown := false
	current := uint64(0)                     // Track cumulative progress across all layers
	layerProgress := make(map[string]uint64) // Track progress per layer ID

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		progressLine := scanner.Text()
		if progressLine == "" {
			continue
		}

		// Parse the progress message
		var progressMsg ProgressMessage
		if err := json.Unmarshal([]byte(html.UnescapeString(progressLine)), &progressMsg); err != nil {
			return "", progressShown, fmt.Errorf("error parsing progress message: %w", err)
		}

		// Handle different message types
		switch progressMsg.Type {
		case "progress":
			// Update the current progress for this layer
			layerID := progressMsg.Layer.ID
			layerProgress[layerID] = progressMsg.Layer.Current

			// Sum all layer progress values
			current = uint64(0)
			for _, layerCurrent := range layerProgress {
				current += layerCurrent
			}

			progress(fmt.Sprintf("Downloaded %s of %s", units.CustomSize("%.2f%s", float64(current), 1000.0, []string{"B", "kB", "MB", "GB", "TB", "PB", "EB", "ZB", "YB"}), units.CustomSize("%.2f%s", float64(progressMsg.Total), 1000.0, []string{"B", "kB", "MB", "GB", "TB", "PB", "EB", "ZB", "YB"})))
			progressShown = true
		case "error":
			return "", progressShown, fmt.Errorf("error pulling model: %s", progressMsg.Message)
		case "success":
			return progressMsg.Message, progressShown, nil
		default:
			return "", progressShown, fmt.Errorf("unknown message type: %s", progressMsg.Type)
		}
	}

	// If we get here, something went wrong
	return "", progressShown, fmt.Errorf("unexpected end of stream while pulling model %s", model)
}

func (c *Client) Push(model string, progress func(string)) (string, bool, error) {
	model = normalizeHuggingFaceModelName(model)
	pushPath := inference.ModelsPrefix + "/" + model + "/push"
	resp, err := c.doRequest(
		http.MethodPost,
		pushPath,
		nil, // Assuming no body is needed for the push request
	)
	if err != nil {
		return "", false, c.handleQueryError(err, pushPath)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", false, fmt.Errorf("pushing %s failed with status %s: %s", model, resp.Status, string(body))
	}

	progressShown := false

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		progressLine := scanner.Text()
		if progressLine == "" {
			continue
		}

		// Parse the progress message
		var progressMsg ProgressMessage
		if err := json.Unmarshal([]byte(html.UnescapeString(progressLine)), &progressMsg); err != nil {
			return "", progressShown, fmt.Errorf("error parsing progress message: %w", err)
		}

		// Handle different message types
		switch progressMsg.Type {
		case "progress":
			progress(progressMsg.Message)
			progressShown = true
		case "error":
			return "", progressShown, fmt.Errorf("error pushing model: %s", progressMsg.Message)
		case "success":
			return progressMsg.Message, progressShown, nil
		default:
			return "", progressShown, fmt.Errorf("unknown message type: %s", progressMsg.Type)
		}
	}

	// If we get here, something went wrong
	return "", progressShown, fmt.Errorf("unexpected end of stream while pushing model %s", model)
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

func (c *Client) ListOpenAI(backend, apiKey string) (dmrm.OpenAIModelList, error) {
	if backend == "" {
		backend = DefaultBackend
	}
	modelsRoute := fmt.Sprintf("%s/%s/v1/models", inference.InferencePrefix, backend)

	// Use doRequestWithAuth to support API key authentication
	resp, err := c.doRequestWithAuth(http.MethodGet, modelsRoute, nil, "openai", apiKey)
	if err != nil {
		return dmrm.OpenAIModelList{}, c.handleQueryError(err, modelsRoute)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return dmrm.OpenAIModelList{}, fmt.Errorf("failed to list models: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return dmrm.OpenAIModelList{}, fmt.Errorf("failed to read response body: %w", err)
	}

	var modelsJson dmrm.OpenAIModelList
	if err := json.Unmarshal(body, &modelsJson); err != nil {
		return modelsJson, fmt.Errorf("failed to unmarshal response body: %w", err)
	}
	return modelsJson, nil
}

func (c *Client) Inspect(model string, remote bool) (dmrm.Model, error) {
	model = normalizeHuggingFaceModelName(model)
	if model != "" {
		if !strings.Contains(strings.Trim(model, "/"), "/") {
			// Do an extra API call to check if the model parameter isn't a model ID.
			modelId, err := c.fullModelID(model)
			if err != nil {
				return dmrm.Model{}, fmt.Errorf("invalid model name: %s", model)
			}
			model = modelId
		}
	}
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
	model = normalizeHuggingFaceModelName(model)
	modelsRoute := inference.InferencePrefix + "/v1/models"
	if !strings.Contains(strings.Trim(model, "/"), "/") {
		// Do an extra API call to check if the model parameter isn't a model ID.
		var err error
		if model, err = c.fullModelID(model); err != nil {
			return dmrm.OpenAIModel{}, fmt.Errorf("invalid model name: %s", model)
		}
	}
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

func (c *Client) fullModelID(id string) (string, error) {
	bodyResponse, err := c.listRaw(inference.ModelsPrefix, "")
	if err != nil {
		return "", err
	}

	var modelsJson []dmrm.Model
	if err := json.Unmarshal(bodyResponse, &modelsJson); err != nil {
		return "", fmt.Errorf("failed to unmarshal response body: %w", err)
	}

	for _, m := range modelsJson {
		if m.ID[7:19] == id || strings.TrimPrefix(m.ID, "sha256:") == id || m.ID == id {
			return m.ID, nil
		}
	}

	return "", fmt.Errorf("model with ID %s not found", id)
}

// Chat performs a chat request and streams the response content with selective markdown rendering.
func (c *Client) Chat(backend, model, prompt, apiKey string, outputFunc func(string), shouldUseMarkdown bool) error {
	return c.ChatWithContext(context.Background(), backend, model, prompt, apiKey, outputFunc, shouldUseMarkdown)
}

// ChatWithContext performs a chat request with context support for cancellation and streams the response content with selective markdown rendering.
func (c *Client) ChatWithContext(ctx context.Context, backend, model, prompt, apiKey string, outputFunc func(string), shouldUseMarkdown bool) error {
	model = normalizeHuggingFaceModelName(model)
	if !strings.Contains(strings.Trim(model, "/"), "/") {
		// Do an extra API call to check if the model parameter isn't a model ID.
		if expanded, err := c.fullModelID(model); err == nil {
			model = expanded
		}
	}

	reqBody := OpenAIChatRequest{
		Model: model,
		Messages: []OpenAIChatMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Stream: true,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("error marshaling request: %w", err)
	}

	var completionsPath string
	if backend != "" {
		completionsPath = inference.InferencePrefix + "/" + backend + "/v1/chat/completions"
	} else {
		completionsPath = inference.InferencePrefix + "/v1/chat/completions"
	}

	resp, err := c.doRequestWithAuthContext(
		ctx,
		http.MethodPost,
		completionsPath,
		bytes.NewReader(jsonData),
		backend,
		apiKey,
	)
	if err != nil {
		return c.handleQueryError(err, completionsPath)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("error response: status=%d body=%s", resp.StatusCode, body)
	}

	type chatPrinterState int
	const (
		chatPrinterNone chatPrinterState = iota
		chatPrinterReasoning
		chatPrinterContent
	)

	printerState := chatPrinterNone
	reasoningFmt := color.New().Add(color.Italic)

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
			return ctx.Err()
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
			return fmt.Errorf("error parsing stream response: %w", err)
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
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading response stream: %w", err)
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

	return nil
}

func (c *Client) Remove(models []string, force bool) (string, error) {
	modelRemoved := ""
	for _, model := range models {
		model = normalizeHuggingFaceModelName(model)
		// Check if not a model ID passed as parameter.
		if !strings.Contains(model, "/") {
			if expanded, err := c.fullModelID(model); err == nil {
				model = expanded
			}
		}

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

	req, err := http.NewRequest(http.MethodGet, path, nil)
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

// doRequest is a helper function that performs HTTP requests and handles 503 responses
func (c *Client) doRequest(method, path string, body io.Reader) (*http.Response, error) {
	return c.doRequestWithAuth(method, path, body, "", "")
}

// doRequestWithAuth is a helper function that performs HTTP requests with optional authentication
func (c *Client) doRequestWithAuth(method, path string, body io.Reader, backend, apiKey string) (*http.Response, error) {
	return c.doRequestWithAuthContext(context.Background(), method, path, body, backend, apiKey)
}

func (c *Client) doRequestWithAuthContext(ctx context.Context, method, path string, body io.Reader, backend, apiKey string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.modelRunner.URL(path), body)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	req.Header.Set("User-Agent", "docker-model-cli/"+Version)

	// Add Authorization header for OpenAI backend
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

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
	source = normalizeHuggingFaceModelName(source)
	// Check if the source is a model ID, and expand it if necessary
	if !strings.Contains(strings.Trim(source, "/"), "/") {
		// Do an extra API call to check if the model parameter might be a model ID
		if expanded, err := c.fullModelID(source); err == nil {
			source = expanded
		}
	}

	// Construct the URL with query parameters
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
