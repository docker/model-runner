package desktop

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	mockdesktop "github.com/docker/model-runner/cmd/cli/mocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

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
