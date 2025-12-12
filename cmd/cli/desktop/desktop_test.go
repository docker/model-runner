package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	mockdesktop "github.com/docker/model-runner/cmd/cli/mocks"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestPullHuggingFaceModel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Test case for pulling a Hugging Face model with mixed case
	modelName := "hf.co/Bartowski/Llama-3.2-1B-Instruct-GGUF"
	expectedLowercase := "hf.co/bartowski/llama-3.2-1b-instruct-gguf"

	mockClient := mockdesktop.NewMockDockerHttpClient(ctrl)
	mockContext := NewContextForMock(mockClient)
	client := New(mockContext)

	mockClient.EXPECT().Do(gomock.Any()).Do(func(req *http.Request) {
		var reqBody models.ModelCreateRequest
		err := json.NewDecoder(req.Body).Decode(&reqBody)
		require.NoError(t, err)
		assert.Equal(t, expectedLowercase, reqBody.From)
	}).Return(&http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"type":"success","message":"Model pulled successfully"}`)),
	}, nil)

	printer := NewSimplePrinter(func(s string) {})
	_, _, err := client.Pull(modelName, printer)
	assert.NoError(t, err)
}

func TestChatHuggingFaceModel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Test case for chatting with a Hugging Face model with mixed case
	modelName := "hf.co/Bartowski/Llama-3.2-1B-Instruct-GGUF"
	expectedLowercase := "hf.co/bartowski/llama-3.2-1b-instruct-gguf"
	prompt := "Hello"

	mockClient := mockdesktop.NewMockDockerHttpClient(ctrl)
	mockContext := NewContextForMock(mockClient)
	client := New(mockContext)

	mockClient.EXPECT().Do(gomock.Any()).Do(func(req *http.Request) {
		var reqBody OpenAIChatRequest
		err := json.NewDecoder(req.Body).Decode(&reqBody)
		require.NoError(t, err)
		assert.Equal(t, expectedLowercase, reqBody.Model)
	}).Return(&http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString("data: {\"choices\":[{\"delta\":{\"content\":\"Hello there!\"}}]}\n")),
	}, nil)

	err := client.Chat(modelName, prompt, []string{}, func(s string) {}, false)
	assert.NoError(t, err)
}

func TestInspectHuggingFaceModel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Test case for inspecting a Hugging Face model with mixed case
	modelName := "hf.co/Bartowski/Llama-3.2-1B-Instruct-GGUF"
	expectedLowercase := "hf.co/bartowski/llama-3.2-1b-instruct-gguf"

	mockClient := mockdesktop.NewMockDockerHttpClient(ctrl)
	mockContext := NewContextForMock(mockClient)
	client := New(mockContext)

	mockClient.EXPECT().Do(gomock.Any()).Do(func(req *http.Request) {
		assert.Contains(t, req.URL.Path, expectedLowercase)
	}).Return(&http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(bytes.NewBufferString(`{
			"id": "sha256:123456789012",
			"tags": ["` + expectedLowercase + `"],
			"created": 1234567890,
			"config": {
				"format": "gguf",
				"quantization": "Q4_K_M",
				"parameters": "1B",
				"architecture": "llama",
				"size": "1.2GB"
			}
		}`)),
	}, nil)

	model, err := client.Inspect(modelName, false)
	assert.NoError(t, err)
	assert.Equal(t, expectedLowercase, model.Tags[0])
}

func TestNonHuggingFaceModel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Test case for a non-Hugging Face model (should not be converted to lowercase)
	modelName := "docker.io/library/llama2"
	mockClient := mockdesktop.NewMockDockerHttpClient(ctrl)
	mockContext := NewContextForMock(mockClient)
	client := New(mockContext)

	mockClient.EXPECT().Do(gomock.Any()).Do(func(req *http.Request) {
		var reqBody models.ModelCreateRequest
		err := json.NewDecoder(req.Body).Decode(&reqBody)
		require.NoError(t, err)
		assert.Equal(t, modelName, reqBody.From)
	}).Return(&http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"type":"success","message":"Model pulled successfully"}`)),
	}, nil)

	printer := NewSimplePrinter(func(s string) {})
	_, _, err := client.Pull(modelName, printer)
	assert.NoError(t, err)
}

func TestPushHuggingFaceModel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Test case for pushing a Hugging Face model with mixed case
	modelName := "hf.co/Bartowski/Llama-3.2-1B-Instruct-GGUF"
	expectedLowercase := "hf.co/bartowski/llama-3.2-1b-instruct-gguf"

	mockClient := mockdesktop.NewMockDockerHttpClient(ctrl)
	mockContext := NewContextForMock(mockClient)
	client := New(mockContext)

	mockClient.EXPECT().Do(gomock.Any()).Do(func(req *http.Request) {
		assert.Contains(t, req.URL.Path, expectedLowercase)
	}).Return(&http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"type":"success","message":"Model pushed successfully"}`)),
	}, nil)

	printer := NewSimplePrinter(func(s string) {})
	_, _, err := client.Push(modelName, printer)
	assert.NoError(t, err)
}

func TestRemoveHuggingFaceModel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Test case for removing a Hugging Face model with mixed case
	modelName := "hf.co/Bartowski/Llama-3.2-1B-Instruct-GGUF"
	expectedLowercase := "hf.co/bartowski/llama-3.2-1b-instruct-gguf"

	mockClient := mockdesktop.NewMockDockerHttpClient(ctrl)
	mockContext := NewContextForMock(mockClient)
	client := New(mockContext)

	mockClient.EXPECT().Do(gomock.Any()).Do(func(req *http.Request) {
		assert.Contains(t, req.URL.Path, expectedLowercase)
	}).Return(&http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString("Model removed successfully")),
	}, nil)

	_, err := client.Remove([]string{modelName}, false)
	assert.NoError(t, err)
}

func TestTagHuggingFaceModel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Test case for tagging a Hugging Face model with mixed case
	sourceModel := "hf.co/Bartowski/Llama-3.2-1B-Instruct-GGUF"
	expectedLowercase := "hf.co/bartowski/llama-3.2-1b-instruct-gguf"
	targetRepo := "myrepo"
	targetTag := "latest"

	mockClient := mockdesktop.NewMockDockerHttpClient(ctrl)
	mockContext := NewContextForMock(mockClient)
	client := New(mockContext)

	mockClient.EXPECT().Do(gomock.Any()).Do(func(req *http.Request) {
		assert.Contains(t, req.URL.Path, expectedLowercase)
	}).Return(&http.Response{
		StatusCode: http.StatusCreated,
		Body:       io.NopCloser(bytes.NewBufferString("Tag created successfully")),
	}, nil)

	assert.NoError(t, client.Tag(sourceModel, targetRepo, targetTag))
}

func TestInspectOpenAIHuggingFaceModel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Test case for inspecting a Hugging Face model with mixed case
	modelName := "hf.co/Bartowski/Llama-3.2-1B-Instruct-GGUF"
	expectedLowercase := "hf.co/bartowski/llama-3.2-1b-instruct-gguf"

	mockClient := mockdesktop.NewMockDockerHttpClient(ctrl)
	mockContext := NewContextForMock(mockClient)
	client := New(mockContext)

	mockClient.EXPECT().Do(gomock.Any()).Do(func(req *http.Request) {
		assert.Contains(t, req.URL.Path, expectedLowercase)
	}).Return(&http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(bytes.NewBufferString(`{
			"id": "` + expectedLowercase + `",
			"object": "model",
			"created": 1234567890,
			"owned_by": "organization"
		}`)),
	}, nil)

	model, err := client.InspectOpenAI(modelName)
	assert.NoError(t, err)
	assert.Equal(t, expectedLowercase, model.ID)
}

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

func TestPullRetryOn5xxError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	modelName := "test-model"
	mockClient := mockdesktop.NewMockDockerHttpClient(ctrl)
	mockContext := NewContextForMock(mockClient)
	client := New(mockContext)

	// First attempt fails with 500, second succeeds
	gomock.InOrder(
		mockClient.EXPECT().Do(gomock.Any()).Return(&http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString("Internal server error")),
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
	assert.Contains(t, err.Error(), "failed to download after 3 retries")
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
