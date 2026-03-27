package desktop

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	mockdesktop "github.com/docker/model-runner/cmd/cli/mocks"
	"github.com/docker/model-runner/pkg/distribution/distribution"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// errorReadCloser is an io.ReadCloser whose Read always returns an error.
type errorReadCloser struct{ err error }

func (e *errorReadCloser) Read(_ []byte) (int, error) { return 0, e.err }
func (e *errorReadCloser) Close() error               { return nil }

func TestPullRetryOnNetworkError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	modelName := "test-model"
	mockClient := mockdesktop.NewMockDockerHttpClient(ctrl)
	mockContext := NewContextForMock(mockClient)
	client := New(mockContext)

	// First two attempts fail with network error, third succeeds
	gomock.InOrder(
		mockClient.EXPECT().Do(gomock.Any()).Return(nil, io.ErrUnexpectedEOF),
		mockClient.EXPECT().Do(gomock.Any()).Return(nil, io.ErrUnexpectedEOF),
		mockClient.EXPECT().Do(gomock.Any()).Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(`{"type":"success","message":"Model pulled successfully"}`)),
		}, nil),
	)

	printer := NewSimplePrinter(func(s string) {})
	_, _, err := client.Pull(modelName, printer)
	assert.NoError(t, err)
}

func TestPullNoRetryOn4xxError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	modelName := "test-model"
	mockClient := mockdesktop.NewMockDockerHttpClient(ctrl)
	mockContext := NewContextForMock(mockClient)
	client := New(mockContext)

	// Should not retry on 404 (client error)
	mockClient.EXPECT().Do(gomock.Any()).Return(&http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(bytes.NewBufferString("Model not found")),
	}, nil).Times(1)

	printer := NewSimplePrinter(func(s string) {})
	_, _, err := client.Pull(modelName, printer)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Model not found")
}

func TestPullNoRetryOn500Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	modelName := "test-model"
	mockClient := mockdesktop.NewMockDockerHttpClient(ctrl)
	mockContext := NewContextForMock(mockClient)
	client := New(mockContext)

	// 500 is not retried (deterministic server error), so only 1 call.
	mockClient.EXPECT().Do(gomock.Any()).Return(&http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(bytes.NewBufferString("Internal server error")),
	}, nil).Times(1)

	printer := NewSimplePrinter(func(s string) {})
	_, _, err := client.Pull(modelName, printer)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Internal server error")
}

func TestPullNoRetryOn422Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	modelName := "test-model"
	mockClient := mockdesktop.NewMockDockerHttpClient(ctrl)
	mockContext := NewContextForMock(mockClient)
	client := New(mockContext)

	// 422 (unsupported media type) must not be retried.
	unsupportedMsg := `error while pulling model: config type "v0.3" is not supported` +
		` - try upgrading`
	mockClient.EXPECT().Do(gomock.Any()).Return(&http.Response{
		StatusCode: http.StatusUnprocessableEntity,
		Body:       io.NopCloser(bytes.NewBufferString(unsupportedMsg)),
	}, nil).Times(1)

	printer := NewSimplePrinter(func(s string) {})
	_, _, err := client.Pull(modelName, printer)
	require.Error(t, err)
	// The sentinel must be preserved so callers can use errors.Is.
	assert.True(t, errors.Is(err, distribution.ErrUnsupportedMediaType))
}

func TestPullRetriesOnTransientGatewayErrors(t *testing.T) {
	// 502 and 504 are transient gateway/proxy errors and should be retried.
	// Note: 503 is intercepted by doRequestWithAuthContext as ErrServiceUnavailable
	// and is covered separately by TestPullRetryOnServiceUnavailable.
	transientCodes := []struct {
		code int
		name string
		body string
	}{
		{http.StatusBadGateway, "502 Bad Gateway", "Bad Gateway"},
		{http.StatusGatewayTimeout, "504 Gateway Timeout", "Gateway Timeout"},
	}

	for _, tc := range transientCodes {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mockdesktop.NewMockDockerHttpClient(ctrl)
			mockContext := NewContextForMock(mockClient)
			client := New(mockContext)

			// First attempt fails with the transient error, second succeeds.
			gomock.InOrder(
				mockClient.EXPECT().Do(gomock.Any()).Return(&http.Response{
					StatusCode: tc.code,
					Body:       io.NopCloser(bytes.NewBufferString(tc.body)),
				}, nil),
				mockClient.EXPECT().Do(gomock.Any()).Return(&http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(bytes.NewBufferString(
						`{"type":"success","message":"Model pulled successfully"}`,
					)),
				}, nil),
			)

			printer := NewSimplePrinter(func(s string) {})
			_, _, err := client.Pull("test-model", printer)
			assert.NoError(t, err)
		})
	}
}

func TestPullRetryOnServiceUnavailable(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	modelName := "test-model"
	mockClient := mockdesktop.NewMockDockerHttpClient(ctrl)
	mockContext := NewContextForMock(mockClient)
	client := New(mockContext)

	// First attempt fails with 503 (converted to ErrServiceUnavailable), second succeeds
	// Note: 503 is handled specially in doRequestWithAuthContext and returns ErrServiceUnavailable
	gomock.InOrder(
		mockClient.EXPECT().Do(gomock.Any()).Return(&http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(bytes.NewBufferString("Service temporarily unavailable")),
		}, nil),
		mockClient.EXPECT().Do(gomock.Any()).Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(`{"type":"success","message":"Model pulled successfully"}`)),
		}, nil),
	)

	printer := NewSimplePrinter(func(s string) {})
	_, _, err := client.Pull(modelName, printer)
	assert.NoError(t, err)
}

func TestPullMaxRetriesExhausted(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	modelName := "test-model"
	mockClient := mockdesktop.NewMockDockerHttpClient(ctrl)
	mockContext := NewContextForMock(mockClient)
	client := New(mockContext)

	// All 4 attempts (1 initial + 3 retries) fail with network error
	mockClient.EXPECT().Do(gomock.Any()).Return(nil, io.EOF).Times(4)

	printer := NewSimplePrinter(func(s string) {})
	_, _, err := client.Pull(modelName, printer)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "download failed after 3 retries")
}

func TestPushRetryOnNetworkError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	modelName := "test-model"
	mockClient := mockdesktop.NewMockDockerHttpClient(ctrl)
	mockContext := NewContextForMock(mockClient)
	client := New(mockContext)

	// First attempt fails with network error, second succeeds
	gomock.InOrder(
		mockClient.EXPECT().Do(gomock.Any()).Return(nil, io.ErrUnexpectedEOF),
		mockClient.EXPECT().Do(gomock.Any()).Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(`{"type":"success","message":"Model pushed successfully"}`)),
		}, nil),
	)

	printer := NewSimplePrinter(func(s string) {})
	_, _, err := client.Push(modelName, printer)
	assert.NoError(t, err)
}

// mockTool is a minimal ClientTool for testing.
type mockTool struct{ name string }

func (m *mockTool) Name() string { return m.name }
func (m *mockTool) Schema() Tool {
	return Tool{Type: "function", Function: ToolFunction{Name: m.name}}
}
func (m *mockTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	return "result", nil
}

// jinjaErrorBody is the 500 response body that a model with an incompatible chat
// template returns when tools are included in the request.
const jinjaErrorBody = `{"error":{"code":500,"message":"Jinja Exception: Conversation roles must alternate user/assistant/user/assistant/...","type":"server_error"}}`

// sseResponse builds a minimal SSE response body with a single content chunk.
func sseResponse(content string) string {
	return "data: {\"choices\":[{\"delta\":{\"content\":\"" + content + "\"},\"finish_reason\":null,\"index\":0}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\",\"index\":0}]}\n\n" +
		"data: [DONE]\n\n"
}

// TestChatWithMessagesContext_JinjaFallbackNoTools verifies that when a model returns a
// 500 Jinja template error (because it doesn't support tool calling), the client
// retries the request without tools and succeeds.
func TestChatWithMessagesContext_JinjaFallbackNoTools(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mockdesktop.NewMockDockerHttpClient(ctrl)
	client := New(NewContextForMock(mockClient))

	gomock.InOrder(
		// First call includes tools → model returns Jinja error
		mockClient.EXPECT().Do(gomock.Any()).Return(&http.Response{
			StatusCode: http.StatusInternalServerError,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString(jinjaErrorBody)),
		}, nil),
		// Retry without tools → model responds successfully
		mockClient.EXPECT().Do(gomock.Any()).Return(&http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(bytes.NewBufferString(sseResponse("Hello!"))),
		}, nil),
	)

	var output string
	resp, err := client.ChatWithMessagesContext(
		t.Context(), "gemma3", nil, "hi", nil,
		func(s string) { output += s },
		false,
		&mockTool{name: "web_search"},
	)
	require.NoError(t, err)
	assert.Equal(t, "Hello!", resp)
	assert.Equal(t, "Hello!", output)
}

// toolCallSSEResponse returns an SSE stream that emits a tool_call finish and then
// a single tool call for the given tool name with empty arguments.
func toolCallSSEResponse(toolName string) string {
	return `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call1","type":"function","function":{"name":"` + toolName + `","arguments":"{}"}}]},"finish_reason":null,"index":0}]}` + "\n\n" +
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls","index":0}]}` + "\n\n" +
		"data: [DONE]\n\n"
}

// TestChatWithMessagesContext_JinjaFallbackAfterToolCall verifies that when a model
// successfully executes a tool call but then fails with a Jinja error when the tool
// result is sent back (because it doesn't support the "tool" role), the client resets
// to the original messages and retries without tools.
func TestChatWithMessagesContext_JinjaFallbackAfterToolCall(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mockdesktop.NewMockDockerHttpClient(ctrl)
	client := New(NewContextForMock(mockClient))

	gomock.InOrder(
		// First call with tools → model responds with a tool_call
		mockClient.EXPECT().Do(gomock.Any()).Return(&http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(bytes.NewBufferString(toolCallSSEResponse("web_search"))),
		}, nil),
		// Second call with tool results → model returns Jinja error (can't handle "tool" role)
		mockClient.EXPECT().Do(gomock.Any()).Return(&http.Response{
			StatusCode: http.StatusInternalServerError,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString(jinjaErrorBody)),
		}, nil),
		// Third call: reset to original messages, no tools → model responds successfully
		mockClient.EXPECT().Do(gomock.Any()).Return(&http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(bytes.NewBufferString(sseResponse("Here is the news."))),
		}, nil),
	)

	var output string
	resp, err := client.ChatWithMessagesContext(
		t.Context(), "gemma3", nil, "Tell me the news", nil,
		func(s string) { output += s },
		false,
		&mockTool{name: "web_search"},
	)
	require.NoError(t, err)
	assert.Equal(t, "Here is the news.", resp)
	assert.Equal(t, "Here is the news.", output)
}

// TestChatWithMessagesContext_Non500ErrorNotRetried verifies that non-Jinja 500 errors
// are not silently retried without tools.
func TestChatWithMessagesContext_Non500ErrorNotRetried(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mockdesktop.NewMockDockerHttpClient(ctrl)
	client := New(NewContextForMock(mockClient))

	// Only one call should be made — no retry for unrelated errors.
	mockClient.EXPECT().Do(gomock.Any()).Return(&http.Response{
		StatusCode: http.StatusInternalServerError,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(`{"error":"out of memory"}`)),
	}, nil).Times(1)

	_, err := client.ChatWithMessagesContext(
		t.Context(), "gemma3", nil, "hi", nil,
		func(string) {},
		false,
		&mockTool{name: "web_search"},
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of memory")
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"EOF error", io.EOF, true},
		{"UnexpectedEOF error", io.ErrUnexpectedEOF, true},
		{"connection reset in string", errors.New("some error: connection reset by peer"), true},
		{"timeout in string", errors.New("operation failed: i/o timeout"), true},
		{"connection refused", errors.New("dial tcp: connection refused"), true},
		{"broken pipe", errors.New("write: broken pipe"), true},
		{"network unreachable", errors.New("network is unreachable"), true},
		{"no such host", errors.New("lookup failed: no such host"), true},
		{"no route to host", errors.New("read tcp: no route to host"), true},
		{"generic non-retryable error", errors.New("a generic non-retryable error"), false},
		{"service unavailable error", ErrServiceUnavailable, true},
		{"deadline exceeded", context.DeadlineExceeded, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryableError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsTemplateIncompatibleError(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected bool
	}{
		{"empty body", "", false},
		{"jinja error", `{"error":"Jinja template error: unsupported role"}`, true},
		{"template error", `{"error":"template does not support tools"}`, true},
		{"generic error", `{"error":"out of memory"}`, false},
		{"jinja in message", "model failed: Jinja exception in chat template", true},
		{"Template capitalized", `{"error":"Template rendering failed"}`, true},
		{"JINJA uppercase", `{"error":"JINJA EXCEPTION"}`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTemplateIncompatibleError([]byte(tt.body))
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPullBodyReadFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mockdesktop.NewMockDockerHttpClient(ctrl)
	mockContext := NewContextForMock(mockClient)
	client := New(mockContext)

	// Response body read fails. Use a non-retryable 500 status so the test
	// completes in a single attempt.
	mockClient.EXPECT().Do(gomock.Any()).Return(&http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       &errorReadCloser{err: errors.New("connection reset")},
	}, nil).Times(1)

	printer := NewSimplePrinter(func(s string) {})
	_, _, err := client.Pull("test-model", printer)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read response body")
}

func TestDisplayProgressNonJSONLines(t *testing.T) {
	// Simulate a proxy returning an HTML error page instead of a progress stream.
	htmlBody := "<html><body><h1>502 Bad Gateway</h1></body></html>\n"
	printer := NewSimplePrinter(func(string) {})
	_, _, err := DisplayProgress(strings.NewReader(htmlBody), printer)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected response from server")
	assert.Contains(t, err.Error(), "502 Bad Gateway")
}

func TestDisplayProgressMixedContent(t *testing.T) {
	// Valid progress followed by some unparseable lines: the valid progress
	// should be honoured and no error returned for the stray lines.
	body := `{"type":"success","message":"Model pulled successfully"}` + "\n" +
		"<html>some extra garbage</html>\n"
	printer := NewSimplePrinter(func(string) {})
	msg, _, err := DisplayProgress(strings.NewReader(body), printer)
	require.NoError(t, err)
	assert.Equal(t, "Model pulled successfully", msg)
}
