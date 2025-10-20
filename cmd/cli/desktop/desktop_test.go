package desktop

import (
	"bytes"
	"encoding/json"
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
	expectedLowercase := "hf.co/bartowski/llama-3.2-1b-instruct-gguf:latest"

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

	_, _, err := client.Pull(modelName, false, func(s string) {})
	assert.NoError(t, err)
}

func TestChatHuggingFaceModel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Test case for chatting with a Hugging Face model with mixed case
	modelName := "hf.co/Bartowski/Llama-3.2-1B-Instruct-GGUF"
	expectedLowercase := "hf.co/bartowski/llama-3.2-1b-instruct-gguf:latest"
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

	err := client.Chat("", modelName, prompt, "", func(s string) {}, false)
	assert.NoError(t, err)
}

func TestInspectHuggingFaceModel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Test case for inspecting a Hugging Face model with mixed case
	modelName := "hf.co/Bartowski/Llama-3.2-1B-Instruct-GGUF"
	expectedLowercase := "hf.co/bartowski/llama-3.2-1b-instruct-gguf:latest"

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
	expectedWithTag := "docker.io/library/llama2:latest"
	mockClient := mockdesktop.NewMockDockerHttpClient(ctrl)
	mockContext := NewContextForMock(mockClient)
	client := New(mockContext)

	mockClient.EXPECT().Do(gomock.Any()).Do(func(req *http.Request) {
		var reqBody models.ModelCreateRequest
		err := json.NewDecoder(req.Body).Decode(&reqBody)
		require.NoError(t, err)
		assert.Equal(t, expectedWithTag, reqBody.From)
	}).Return(&http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"type":"success","message":"Model pulled successfully"}`)),
	}, nil)

	_, _, err := client.Pull(modelName, false, func(s string) {})
	assert.NoError(t, err)
}

func TestPushHuggingFaceModel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Test case for pushing a Hugging Face model with mixed case
	modelName := "hf.co/Bartowski/Llama-3.2-1B-Instruct-GGUF"
	expectedLowercase := "hf.co/bartowski/llama-3.2-1b-instruct-gguf:latest"

	mockClient := mockdesktop.NewMockDockerHttpClient(ctrl)
	mockContext := NewContextForMock(mockClient)
	client := New(mockContext)

	mockClient.EXPECT().Do(gomock.Any()).Do(func(req *http.Request) {
		assert.Contains(t, req.URL.Path, expectedLowercase)
	}).Return(&http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"type":"success","message":"Model pushed successfully"}`)),
	}, nil)

	_, _, err := client.Push(modelName, func(s string) {})
	assert.NoError(t, err)
}

func TestRemoveHuggingFaceModel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Test case for removing a Hugging Face model with mixed case
	modelName := "hf.co/Bartowski/Llama-3.2-1B-Instruct-GGUF"
	expectedLowercase := "hf.co/bartowski/llama-3.2-1b-instruct-gguf:latest"

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
	expectedLowercase := "hf.co/bartowski/llama-3.2-1b-instruct-gguf:latest"
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
	expectedLowercase := "hf.co/bartowski/llama-3.2-1b-instruct-gguf:latest"

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
