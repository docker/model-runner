package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/docker/model-runner/cmd/cli/desktop"
)

const (
	exaMCPURL         = "https://mcp.exa.ai/mcp"
	searchTimeout     = 25 * time.Second
	defaultNumResults = 8

	// youcomFreeMCPURL is You.com's keyless MCP endpoint (no account or key needed).
	youcomFreeMCPURL = "https://api.you.com/mcp?profile=free"
	// youcomMCPURL is used instead when YDC_API_KEY is set, for higher limits.
	youcomMCPURL = "https://api.you.com/mcp"
	// youcomUserAgent is sent on You.com requests only.
	youcomUserAgent = "youdotcom-integration/docker-model-runner"
)

// Supported web search providers.
const (
	ProviderExa    = "exa"
	ProviderYouCom = "youcom"
)

// WebSearchProviders lists the accepted values for the web search provider.
var WebSearchProviders = []string{ProviderExa, ProviderYouCom}

// ValidateWebSearchProvider returns an error if provider is not supported.
// An empty provider is accepted and means the default (Exa).
func ValidateWebSearchProvider(provider string) error {
	switch provider {
	case "", ProviderExa, ProviderYouCom:
		return nil
	default:
		return fmt.Errorf("unknown web search provider %q (supported: %s)", provider, strings.Join(WebSearchProviders, ", "))
	}
}

// WebSearchTool implements web search via a hosted MCP search endpoint.
// Exa is the default. Set Provider to "youcom" to use You.com instead.
type WebSearchTool struct {
	// Provider selects the search backend: "exa" (default) or "youcom".
	Provider string
	// endpoint overrides the provider URL (tests only).
	endpoint string
	// apiKey overrides YDC_API_KEY lookup (tests only).
	apiKey *string
}

// Name returns the tool name.
func (w *WebSearchTool) Name() string { return "web_search" }

// Schema returns the OpenAI tool definition for web search.
func (w *WebSearchTool) Schema() desktop.Tool {
	return desktop.Tool{
		Type: "function",
		Function: desktop.ToolFunction{
			Name:        w.Name(),
			Description: fmt.Sprintf("Search the web for current information up to %d results. Use this when you need up-to-date information that may not be in your training data.", defaultNumResults),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The search query",
					},
				},
				"required": []string{"query"},
			},
		},
	}
}

func (w *WebSearchTool) youcomAPIKey() string {
	if w.apiKey != nil {
		return *w.apiKey
	}
	return strings.TrimSpace(os.Getenv("YDC_API_KEY"))
}

// buildRequest returns the endpoint URL, JSON-RPC body and extra headers for the selected provider.
func (w *WebSearchTool) buildRequest(query string) (string, map[string]any, map[string]string, error) {
	switch w.Provider {
	case "", ProviderExa:
		url := exaMCPURL
		if w.endpoint != "" {
			url = w.endpoint
		}
		return url, map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]any{
				"name": "web_search_exa",
				"arguments": map[string]any{
					"query":      query,
					"numResults": defaultNumResults,
					"type":       "auto",
					"livecrawl":  "fallback",
				},
			},
		}, nil, nil
	case ProviderYouCom:
		headers := map[string]string{"User-Agent": youcomUserAgent}
		url := youcomFreeMCPURL
		if key := w.youcomAPIKey(); key != "" {
			url = youcomMCPURL
			headers["Authorization"] = "Bearer " + key
		}
		if w.endpoint != "" {
			url = w.endpoint
		}
		return url, map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]any{
				"name": "you-search",
				"arguments": map[string]any{
					"query": query,
					"count": defaultNumResults,
				},
			},
		}, headers, nil
	default:
		return "", nil, nil, ValidateWebSearchProvider(w.Provider)
	}
}

// Execute performs the web search using the selected provider's MCP endpoint.
func (w *WebSearchTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return "", fmt.Errorf("query parameter is required")
	}

	url, reqBody, headers, err := w.buildRequest(query)
	if err != nil {
		return "", err
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("executing search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("search API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	return parseMCPToolResponse(string(body))
}

type mcpToolResponse struct {
	Result *struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	} `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// parseMCPToolResponse extracts the tool result text from an MCP tools/call
// response. The response may be plain JSON or SSE (text/event-stream). An SSE
// stream can carry notification messages before the result, so each event is
// parsed and the one holding a result (or error) is used.
func parseMCPToolResponse(responseText string) (string, error) {
	var payloads []string
	for _, line := range strings.Split(responseText, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "data:") {
			payloads = append(payloads, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if len(payloads) == 0 {
		payloads = []string{responseText}
	}

	var parseErr error
	for _, payload := range payloads {
		var mcpResp mcpToolResponse
		if err := json.Unmarshal([]byte(payload), &mcpResp); err != nil {
			parseErr = err
			continue
		}
		if mcpResp.Error != nil {
			return "", fmt.Errorf("search API error %d: %s", mcpResp.Error.Code, mcpResp.Error.Message)
		}
		if mcpResp.Result == nil {
			// Notification or other non-result message.
			continue
		}
		text := ""
		if len(mcpResp.Result.Content) > 0 {
			text = mcpResp.Result.Content[0].Text
		}
		if mcpResp.Result.IsError {
			return "", fmt.Errorf("search failed: %s", text)
		}
		if text == "" {
			return "No search results found.", nil
		}
		return text, nil
	}
	if parseErr != nil {
		return "", fmt.Errorf("parsing response: %w", parseErr)
	}
	return "No search results found.", nil
}
