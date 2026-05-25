package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/docker/model-runner/cmd/cli/desktop"
)

const (
	exaMCPURL         = "https://mcp.exa.ai/mcp"
	searchTimeout     = 25 * time.Second
	defaultNumResults = 8
)

// WebSearchTool implements web search via Exa's MCP API.
type WebSearchTool struct{}

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

// Execute performs the web search using Exa's MCP endpoint.
func (w *WebSearchTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return "", fmt.Errorf("query parameter is required")
	}

	reqBody := map[string]any{
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
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, exaMCPURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

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

	// Response may be SSE or plain JSON — handle both.
	responseText := string(body)

	// For SSE (text/event-stream), accumulate all data: lines into a single payload.
	var dataLines []string
	for _, line := range strings.Split(responseText, "\n") {
		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		}
	}
	if len(dataLines) > 0 {
		responseText = strings.Join(dataLines, "\n")
	}

	var mcpResp struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(responseText), &mcpResp); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}

	if len(mcpResp.Result.Content) > 0 {
		return mcpResp.Result.Content[0].Text, nil
	}
	return "No search results found.", nil
}
