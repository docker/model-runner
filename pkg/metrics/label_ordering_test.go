package metrics

import (
	"testing"
)

func TestConsistentLabelOrdering(t *testing.T) {
	// Create a metric with labels in different orders
	metric := PrometheusMetric{
		Name: "test_metric",
		Labels: map[string]string{
			"model":   "ai/llama3.2",
			"mode":    "completion",
			"backend": "llama.cpp",
		},
		Value: "123",
	}

	// Format the metric multiple times
	results := make([]string, 10)
	for i := 0; i < 10; i++ {
		results[i] = metric.FormatMetric()
	}

	// All results should be identical
	expected := `test_metric{backend="llama.cpp",mode="completion",model="ai/llama3.2"} 123`
	for i, result := range results {
		if result != expected {
			t.Errorf("Iteration %d: Expected '%s', got '%s'", i, expected, result)
		}
	}

	// Verify all results are the same
	for i := 1; i < len(results); i++ {
		if results[i] != results[0] {
			t.Errorf("Inconsistent ordering: result[0]='%s', result[%d]='%s'", results[0], i, results[i])
		}
	}
}

func TestLabelOrderingWithDifferentKeys(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected string
	}{
		{
			name: "backend_model_mode",
			labels: map[string]string{
				"backend": "llama.cpp",
				"model":   "ai/llama3.2",
				"mode":    "completion",
			},
			expected: `test{backend="llama.cpp",mode="completion",model="ai/llama3.2"} 42`,
		},
		{
			name: "alphabetical_order",
			labels: map[string]string{
				"z_last":  "last",
				"a_first": "first",
				"m_mid":   "middle",
			},
			expected: `test{a_first="first",m_mid="middle",z_last="last"} 42`,
		},
		{
			name: "single_label",
			labels: map[string]string{
				"single": "value",
			},
			expected: `test{single="value"} 42`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metric := PrometheusMetric{
				Name:   "test",
				Labels: tt.labels,
				Value:  "42",
			}

			result := metric.FormatMetric()
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}
