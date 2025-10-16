package commands

import (
	"strings"
	"testing"
	"time"

	dmrm "github.com/docker/model-runner/pkg/inference/models"
)

func TestPrettyPrintOpenAIModels(t *testing.T) {
	modelList := dmrm.OpenAIModelList{
		Object: "list",
		Data: []*dmrm.OpenAIModel{
			{
				ID:      "llama3.2:3b",
				Object:  "model",
				Created: time.Now().Unix() - 3600, // 1 hour ago
				OwnedBy: "docker",
			},
			{
				ID:      "qwen2.5:7b",
				Object:  "model",
				Created: time.Now().Unix() - 86400, // 1 day ago
				OwnedBy: "docker",
			},
		},
	}

	output := prettyPrintOpenAIModels(modelList)

	// Verify it's table format (contains headers)
	if !strings.Contains(output, "MODEL NAME") {
		t.Errorf("Expected output to contain 'MODEL NAME' header, got: %s", output)
	}

	if !strings.Contains(output, "CREATED") {
		t.Errorf("Expected output to contain 'CREATED' header, got: %s", output)
	}

	// Verify model names are in output
	if !strings.Contains(output, "llama3.2:3b") {
		t.Errorf("Expected output to contain 'llama3.2:3b', got: %s", output)
	}

	if !strings.Contains(output, "qwen2.5:7b") {
		t.Errorf("Expected output to contain 'qwen2.5:7b', got: %s", output)
	}

	// Verify time format (should contain "ago")
	if !strings.Contains(output, "ago") {
		t.Errorf("Expected output to contain time format with 'ago', got: %s", output)
	}

	// Verify it's not JSON format
	if strings.Contains(output, "{") || strings.Contains(output, "}") {
		t.Errorf("Expected table format, but got JSON-like output: %s", output)
	}
}

func TestPrettyPrintOpenAIModelsEmpty(t *testing.T) {
	modelList := dmrm.OpenAIModelList{
		Object: "list",
		Data:   []*dmrm.OpenAIModel{},
	}

	output := prettyPrintOpenAIModels(modelList)

	// Should still have headers even with no models
	if !strings.Contains(output, "MODEL NAME") {
		t.Errorf("Expected output to contain 'MODEL NAME' header even with no models, got: %s", output)
	}
}
