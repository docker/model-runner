package tools

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type capturedRequest struct {
	header http.Header
	body   map[string]any
}

func newMCPServer(t *testing.T, response string) (*httptest.Server, *capturedRequest) {
	t.Helper()
	captured := &capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.header = r.Header.Clone()
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &captured.body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(func() {
		srv.Close()
		http.DefaultClient.CloseIdleConnections()
	})
	return srv, captured
}

func toolCall(t *testing.T, body map[string]any) (string, map[string]any) {
	t.Helper()
	params, ok := body["params"].(map[string]any)
	if !ok {
		t.Fatalf("missing params in request body: %v", body)
	}
	name, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]any)
	return name, args
}

func strPtr(s string) *string { return &s }

const youcomSSE = "event: message\n" +
	`data: {"jsonrpc":"2.0","method":"notifications/message","params":{"level":"info","data":"Search successful"}}` + "\n\n" +
	"event: message\n" +
	`data: {"result":{"content":[{"type":"text","text":"{\"results\":{\"web\":[]}}"}]},"jsonrpc":"2.0","id":1}` + "\n\n"

func TestWebSearchYouComKeyless(t *testing.T) {
	srv, captured := newMCPServer(t, youcomSSE)
	tool := &WebSearchTool{Provider: ProviderYouCom, endpoint: srv.URL, apiKey: strPtr("")}

	out, err := tool.Execute(t.Context(), map[string]any{"query": "docker model runner"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != `{"results":{"web":[]}}` {
		t.Errorf("unexpected output: %q", out)
	}
	if got := captured.header.Get("User-Agent"); got != youcomUserAgent {
		t.Errorf("User-Agent = %q, want %q", got, youcomUserAgent)
	}
	if got := captured.header.Get("Authorization"); got != "" {
		t.Errorf("keyless request should not send Authorization, got %q", got)
	}
	name, args := toolCall(t, captured.body)
	if name != "you-search" {
		t.Errorf("tool name = %q, want you-search", name)
	}
	if args["query"] != "docker model runner" {
		t.Errorf("query = %v", args["query"])
	}
	if args["count"] != float64(defaultNumResults) {
		t.Errorf("count = %v, want %d", args["count"], defaultNumResults)
	}
}

func TestWebSearchYouComKeyed(t *testing.T) {
	srv, captured := newMCPServer(t, youcomSSE)
	tool := &WebSearchTool{Provider: ProviderYouCom, endpoint: srv.URL, apiKey: strPtr("test-key")}

	if _, err := tool.Execute(t.Context(), map[string]any{"query": "q"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := captured.header.Get("Authorization"); got != "Bearer test-key" {
		t.Errorf("Authorization = %q, want Bearer test-key", got)
	}
}

func TestWebSearchYouComURLSelection(t *testing.T) {
	keyless := &WebSearchTool{Provider: ProviderYouCom, apiKey: strPtr("")}
	url, _, _, err := keyless.buildRequest("q")
	if err != nil || url != youcomFreeMCPURL {
		t.Errorf("keyless url = %q, err = %v; want %q", url, err, youcomFreeMCPURL)
	}
	keyed := &WebSearchTool{Provider: ProviderYouCom, apiKey: strPtr("k")}
	url, _, _, err = keyed.buildRequest("q")
	if err != nil || url != youcomMCPURL {
		t.Errorf("keyed url = %q, err = %v; want %q", url, err, youcomMCPURL)
	}
}

func TestWebSearchExaDefault(t *testing.T) {
	srv, captured := newMCPServer(t, `data: {"result":{"content":[{"type":"text","text":"exa results"}]}}`+"\n")
	tool := &WebSearchTool{endpoint: srv.URL}

	out, err := tool.Execute(t.Context(), map[string]any{"query": "q"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "exa results" {
		t.Errorf("unexpected output: %q", out)
	}
	if name, _ := toolCall(t, captured.body); name != "web_search_exa" {
		t.Errorf("tool name = %q, want web_search_exa", name)
	}
	if got := captured.header.Get("User-Agent"); got == youcomUserAgent {
		t.Error("Exa requests must not send the You.com User-Agent")
	}
	url, _, _, err := (&WebSearchTool{}).buildRequest("q")
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}
	if url != exaMCPURL {
		t.Errorf("default url = %q, want %q", url, exaMCPURL)
	}
}

func TestWebSearchToolError(t *testing.T) {
	srv, _ := newMCPServer(t, `data: {"result":{"content":[{"type":"text","text":"Error: Failed to perform search. Error code: 401"}],"isError":true},"jsonrpc":"2.0","id":1}`+"\n")
	tool := &WebSearchTool{Provider: ProviderYouCom, endpoint: srv.URL, apiKey: strPtr("bad")}

	_, err := tool.Execute(t.Context(), map[string]any{"query": "q"})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected tool error mentioning 401, got %v", err)
	}
}

func TestParseMCPToolResponse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"plain json", `{"result":{"content":[{"type":"text","text":"hi"}]}}`, "hi", false},
		{"sse with notification first", youcomSSE, `{"results":{"web":[]}}`, false},
		{"empty content", `{"result":{"content":[]}}`, "No search results found.", false},
		{"json-rpc error", `{"error":{"code":-32602,"message":"bad params"}}`, "", true},
		{"invalid", `not json`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMCPToolResponse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateWebSearchProvider(t *testing.T) {
	for _, p := range []string{"", ProviderExa, ProviderYouCom} {
		if err := ValidateWebSearchProvider(p); err != nil {
			t.Errorf("ValidateWebSearchProvider(%q) = %v", p, err)
		}
	}
	if err := ValidateWebSearchProvider("bing"); err == nil {
		t.Error("expected error for unknown provider")
	}
}
