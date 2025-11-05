package commands

import (
	"regexp"
	"strings"
	"testing"

	"github.com/docker/model-runner/pkg/distribution/types"
	dmrm "github.com/docker/model-runner/pkg/inference/models"
)

// Helper to create a test model with minimal required fields
func testModel(id string, tags []string, created int64) dmrm.Model {
	return dmrm.Model{
		ID:      id,
		Tags:    tags,
		Created: created,
		Config: types.Config{
			Parameters:   "7B",
			Quantization: "Q4_0",
			Architecture: "llama",
			Size:         "4.0GB",
		},
	}
}

func TestListModelsSorting(t *testing.T) {
	tests := []struct {
		name          string
		inputModels   []dmrm.Model
		expectedOrder []string // expected display names of all rows (flattened tags) in sorted order
		description   string
	}{
		{
			name: "alphabetical sorting",
			inputModels: []dmrm.Model{
				testModel("sha256:123456789012345678901234567890123456789012345678901234567890abcd", []string{"zebra:latest"}, 1000),
				testModel("sha256:223456789012345678901234567890123456789012345678901234567890abcd", []string{"apple:latest"}, 2000),
				testModel("sha256:323456789012345678901234567890123456789012345678901234567890abcd", []string{"mango:latest"}, 3000),
			},
			expectedOrder: []string{"apple:latest", "mango:latest", "zebra:latest"},
			description:   "Models should be sorted alphabetically by display name",
		},
		{
			name: "case insensitive sorting",
			inputModels: []dmrm.Model{
				testModel("sha256:123456789012345678901234567890123456789012345678901234567890abcd", []string{"Zebra:latest"}, 1000),
				testModel("sha256:223456789012345678901234567890123456789012345678901234567890abcd", []string{"apple:latest"}, 2000),
				testModel("sha256:323456789012345678901234567890123456789012345678901234567890abcd", []string{"Mango:latest"}, 3000),
			},
			expectedOrder: []string{"apple:latest", "Mango:latest", "Zebra:latest"},
			description:   "Sorting should be case-insensitive",
		},
		{
			name: "models with no tags show as <none>",
			inputModels: []dmrm.Model{
				testModel("sha256:123456789012345678901234567890123456789012345678901234567890abcd", []string{"beta:latest"}, 1000),
				testModel("sha256:223456789012345678901234567890123456789012345678901234567890abcd", []string{}, 2000),
				testModel("sha256:323456789012345678901234567890123456789012345678901234567890abcd", []string{"alpha:latest"}, 3000),
			},
			expectedOrder: []string{"<none>", "alpha:latest", "beta:latest"},
			description:   "Models without tags display as <none> and are sorted with other rows",
		},
		{
			name: "multiple models without tags",
			inputModels: []dmrm.Model{
				testModel("sha256:123456789012345678901234567890123456789012345678901234567890abcd", []string{"zebra:latest"}, 1000),
				testModel("sha256:223456789012345678901234567890123456789012345678901234567890abcd", []string{}, 2000),
				testModel("sha256:323456789012345678901234567890123456789012345678901234567890abcd", []string{}, 3000),
				testModel("sha256:423456789012345678901234567890123456789012345678901234567890abcd", []string{"apple:latest"}, 4000),
			},
			expectedOrder: []string{"<none>", "<none>", "apple:latest", "zebra:latest"},
			description:   "Multiple models without tags each create a <none> row, sorted with other rows",
		},
		{
			name: "models with multiple tags create multiple rows",
			inputModels: []dmrm.Model{
				testModel("sha256:123456789012345678901234567890123456789012345678901234567890abcd", []string{"zoo:latest", "animal:v1"}, 1000),
				testModel("sha256:223456789012345678901234567890123456789012345678901234567890abcd", []string{"apple:latest", "fruit:v1"}, 2000),
			},
			expectedOrder: []string{"animal:v1", "apple:latest", "fruit:v1", "zoo:latest"},
			description:   "Each tag creates a separate row, all rows are sorted together",
		},
		{
			name: "models with prefixes",
			inputModels: []dmrm.Model{
				testModel("sha256:123456789012345678901234567890123456789012345678901234567890abcd", []string{"hf.co/model-z:latest"}, 1000),
				testModel("sha256:223456789012345678901234567890123456789012345678901234567890abcd", []string{"hf.co/model-a:latest"}, 2000),
				testModel("sha256:323456789012345678901234567890123456789012345678901234567890abcd", []string{"ai/model-m:latest"}, 3000),
			},
			expectedOrder: []string{"ai/model-m:latest", "hf.co/model-a:latest", "hf.co/model-z:latest"},
			description:   "Should handle models with different prefixes, sorting by display name",
		},
		{
			name: "models with :latest tag vs specific tags",
			inputModels: []dmrm.Model{
				testModel("sha256:123456789012345678901234567890123456789012345678901234567890abcd", []string{"qwen3:0.6B-F16"}, 1000),
				testModel("sha256:223456789012345678901234567890123456789012345678901234567890abcd", []string{"qwen3:latest"}, 2000),
				testModel("sha256:323456789012345678901234567890123456789012345678901234567890abcd", []string{"qwen3:14B-Q6_K"}, 3000),
				testModel("sha256:423456789012345678901234567890123456789012345678901234567890abcd", []string{"qwen3:4B-F16"}, 4000),
			},
			expectedOrder: []string{"qwen3:0.6B-F16", "qwen3:14B-Q6_K", "qwen3:4B-F16", "qwen3:latest"},
			description:   "Models with specific tags should be sorted with :latest",
		},
		{
			name: "base model name sorting - qwen3 vs qwen3-coder",
			inputModels: []dmrm.Model{
				testModel("sha256:123456789012345678901234567890123456789012345678901234567890abcd", []string{"qwen3-coder:latest"}, 1000),
				testModel("sha256:223456789012345678901234567890123456789012345678901234567890abcd", []string{"qwen3:0.6B-F16"}, 2000),
				testModel("sha256:323456789012345678901234567890123456789012345678901234567890abcd", []string{"qwen3:latest"}, 3000),
				testModel("sha256:423456789012345678901234567890123456789012345678901234567890abcd", []string{"qwen3-coder:30B"}, 4000),
			},
			expectedOrder: []string{"qwen3:0.6B-F16", "qwen3:latest", "qwen3-coder:30B", "qwen3-coder:latest"},
			description:   "All qwen3 variants should appear before qwen3-coder variants (base name comparison)",
		},
		{
			name: "complex multi-tag scenario with interspersed rows",
			inputModels: []dmrm.Model{
				// Model 1: has tags that will appear at beginning and end
				testModel("sha256:123456789012345678901234567890123456789012345678901234567890abcd", []string{"zebra:latest", "apple:v1"}, 1000),
				// Model 2: has tags in the middle
				testModel("sha256:223456789012345678901234567890123456789012345678901234567890abcd", []string{"mango:latest", "banana:v2"}, 2000),
			},
			expectedOrder: []string{"apple:v1", "banana:v2", "mango:latest", "zebra:latest"},
			description:   "Tags from different models should be interspersed in sorted order, not grouped by model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call the actual prettyPrintModels function to test the real sorting logic
			output := prettyPrintModels(tt.inputModels)

			// Parse the output to extract model names in order
			actualOrder := extractModelNamesFromOutput(output)

			// Convert expected tags to display names
			expectedDisplayNames := make([]string, len(tt.expectedOrder))
			for i, tag := range tt.expectedOrder {
				if tag == "" {
					expectedDisplayNames[i] = "<none>"
				} else {
					expectedDisplayNames[i] = tag
				}
			}

			// Verify the order
			if len(actualOrder) != len(expectedDisplayNames) {
				t.Fatalf("%s: expected %d models, got %d\nExpected: %v\nActual: %v",
					tt.description, len(expectedDisplayNames), len(actualOrder),
					expectedDisplayNames, actualOrder)
			}

			for i := range expectedDisplayNames {
				if actualOrder[i] != expectedDisplayNames[i] {
					t.Errorf("%s: at position %d, expected %q, got %q\nExpected order: %v\nActual order: %v",
						tt.description, i, expectedDisplayNames[i], actualOrder[i],
						expectedDisplayNames, actualOrder)
				}
			}
		})
	}
}

func TestListModelsEmptyList(t *testing.T) {
	models := []dmrm.Model{}
	output := prettyPrintModels(models)
	actualOrder := extractModelNamesFromOutput(output)
	if len(actualOrder) != 0 {
		t.Errorf("Expected empty list to remain empty, got %d models", len(actualOrder))
	}
}

func TestListModelsSingleModel(t *testing.T) {
	models := []dmrm.Model{
		{
			ID:      "sha256:123456789012345678901234567890123456789012345678901234567890abcd",
			Tags:    []string{"single:latest"},
			Created: 1000,
			Config: types.Config{
				Parameters:   "7B",
				Quantization: "Q4_0",
				Architecture: "llama",
				Size:         "4.0GB",
			},
		},
	}
	output := prettyPrintModels(models)
	actualOrder := extractModelNamesFromOutput(output)
	if len(actualOrder) != 1 || actualOrder[0] != "single:latest" {
		t.Errorf("Single model should remain unchanged, got %v", actualOrder)
	}
}

// extractModelNamesFromOutput parses the table output and extracts model names in order
func extractModelNamesFromOutput(output string) []string {
	var modelNames []string
	lines := strings.Split(output, "\n")

	// Skip header lines and find data rows
	// The table has a header line followed by data rows
	inDataSection := false
	for _, line := range lines {
		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Check if this is the header line
		if strings.Contains(line, "MODEL NAME") {
			inDataSection = true
			continue
		}

		if !inDataSection {
			continue
		}

		// Extract the first column (model name)
		// The table uses multiple spaces as column separator
		fields := regexp.MustCompile(`\s{2,}`).Split(strings.TrimSpace(line), -1)
		if len(fields) > 0 && fields[0] != "" {
			modelNames = append(modelNames, fields[0])
		}
	}

	return modelNames
}

func TestPrettyPrintModelsWithSortedInput(t *testing.T) {
	// This test verifies that prettyPrintModels correctly handles sorted models
	models := []dmrm.Model{
		{
			ID:      "sha256:123456789012345678901234567890123456789012345678901234567890abcd",
			Tags:    []string{"ai/apple:latest"},
			Created: 1000,
			Config: types.Config{
				Parameters:   "7B",
				Quantization: "Q4_0",
				Architecture: "llama",
				Size:         "4.0GB",
			},
		},
		{
			ID:      "sha256:223456789012345678901234567890123456789012345678901234567890abcd",
			Tags:    []string{"ai/banana:v1"},
			Created: 2000,
			Config: types.Config{
				Parameters:   "13B",
				Quantization: "Q4_K_M",
				Architecture: "llama",
				Size:         "8.0GB",
			},
		},
	}

	output := prettyPrintModels(models)

	// Verify output contains both models
	if !strings.Contains(output, "apple") {
		t.Error("Expected output to contain 'apple'")
	}
	if !strings.Contains(output, "banana") {
		t.Error("Expected output to contain 'banana'")
	}

	// Verify apple appears before banana (alphabetical order)
	applePos := strings.Index(output, "apple")
	bananaPos := strings.Index(output, "banana")
	if applePos == -1 || bananaPos == -1 {
		t.Fatal("Could not find model names in output")
	}
	if applePos > bananaPos {
		t.Error("Models are not in alphabetical order in output")
	}
}

func TestPrettyPrintModelsWithMultipleTags(t *testing.T) {
	// This test verifies that tags within a model are sorted correctly
	models := []dmrm.Model{
		{
			ID:      "sha256:123456789012345678901234567890123456789012345678901234567890abcd",
			Tags:    []string{"qwen3:8B-Q4_K_M", "qwen3:latest", "qwen3:0.6B-F16"},
			Created: 1000,
			Config: types.Config{
				Parameters:   "8B",
				Quantization: "Q4_K_M",
				Architecture: "qwen3",
				Size:         "4.68GB",
			},
		},
	}

	output := prettyPrintModels(models)

	// Find positions of each tag display
	qwen3LatestPos := strings.Index(output, "qwen3:latest")
	qwen3_0_6Pos := strings.Index(output, "qwen3:0.6B-F16")
	qwen3_8Pos := strings.Index(output, "qwen3:8B-Q4_K_M")

	if qwen3LatestPos == -1 {
		t.Error("Expected output to contain 'qwen3:latest'")
	}
	if qwen3_0_6Pos == -1 {
		t.Error("Expected output to contain 'qwen3:0.6B-F16'")
	}
	if qwen3_8Pos == -1 {
		t.Error("Expected output to contain 'qwen3:8B-Q4_K_M'")
	}

	// Verify tags are in correct order: qwen3:0.6B-F16 < qwen3:8B-Q4_K_M < qwen3:latest
	if qwen3_0_6Pos > qwen3_8Pos {
		t.Error("'qwen3:0.6B-F16' should appear before 'qwen3:8B-Q4_K_M'")
	}
	if qwen3_8Pos > qwen3LatestPos {
		t.Error("'qwen3:8B-Q4_K_M' should appear before 'qwen3:latest'")
	}
}
