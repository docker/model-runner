package metrics

import (
	"testing"
)

func TestPrometheusParser_ParseMetrics(t *testing.T) {
	parser := NewPrometheusParser()

	input := `# HELP http_requests_total Total number of HTTP requests
# TYPE http_requests_total counter
http_requests_total{method="get",code="200"} 1027
http_requests_total{method="post",code="200"} 3
http_requests_total{method="post",code="400"} 3

# HELP memory_usage_bytes Memory usage in bytes
# TYPE memory_usage_bytes gauge
memory_usage_bytes 1234567890

# Simple metric without labels
simple_metric 42
`

	metrics, err := parser.ParseMetrics(input)
	if err != nil {
		t.Fatalf("Failed to parse metrics: %v", err)
	}

	if len(metrics) != 5 {
		t.Errorf("Expected 5 metrics, got %d", len(metrics))
	}

	// Test first metric with labels
	metric := metrics[0]
	if metric.Name != "http_requests_total" {
		t.Errorf("Expected name 'http_requests_total', got '%s'", metric.Name)
	}
	if metric.Value != "1027" {
		t.Errorf("Expected value '1027', got '%s'", metric.Value)
	}
	if metric.Help != "Total number of HTTP requests" {
		t.Errorf("Expected help 'Total number of HTTP requests', got '%s'", metric.Help)
	}
	if metric.Type != "counter" {
		t.Errorf("Expected type 'counter', got '%s'", metric.Type)
	}
	if len(metric.Labels) != 2 {
		t.Errorf("Expected 2 labels, got %d", len(metric.Labels))
	}
	if metric.Labels["method"] != "get" {
		t.Errorf("Expected method='get', got '%s'", metric.Labels["method"])
	}
	if metric.Labels["code"] != "200" {
		t.Errorf("Expected code='200', got '%s'", metric.Labels["code"])
	}

	// Test simple metric without labels
	simpleMetric := metrics[4]
	if simpleMetric.Name != "simple_metric" {
		t.Errorf("Expected name 'simple_metric', got '%s'", simpleMetric.Name)
	}
	if simpleMetric.Value != "42" {
		t.Errorf("Expected value '42', got '%s'", simpleMetric.Value)
	}
	if len(simpleMetric.Labels) != 0 {
		t.Errorf("Expected 0 labels, got %d", len(simpleMetric.Labels))
	}
}

func TestPrometheusMetric_AddLabels(t *testing.T) {
	metric := PrometheusMetric{
		Name:   "test_metric",
		Labels: map[string]string{"existing": "value"},
		Value:  "123",
	}

	additionalLabels := map[string]string{
		"backend": "llama.cpp",
		"model":   "test-model",
	}

	metric.AddLabels(additionalLabels)

	if len(metric.Labels) != 3 {
		t.Errorf("Expected 3 labels, got %d", len(metric.Labels))
	}

	if metric.Labels["existing"] != "value" {
		t.Errorf("Expected existing='value', got '%s'", metric.Labels["existing"])
	}
	if metric.Labels["backend"] != "llama.cpp" {
		t.Errorf("Expected backend='llama.cpp', got '%s'", metric.Labels["backend"])
	}
	if metric.Labels["model"] != "test-model" {
		t.Errorf("Expected model='test-model', got '%s'", metric.Labels["model"])
	}
}

func TestPrometheusMetric_FormatMetric(t *testing.T) {
	// Test metric without labels
	metric1 := PrometheusMetric{
		Name:  "simple_metric",
		Value: "42",
	}

	expected1 := "simple_metric 42"
	result1 := metric1.FormatMetric()
	if result1 != expected1 {
		t.Errorf("Expected '%s', got '%s'", expected1, result1)
	}

	// Test metric with labels
	metric2 := PrometheusMetric{
		Name: "labeled_metric",
		Labels: map[string]string{
			"backend": "llama.cpp",
			"model":   "test-model",
		},
		Value: "123",
	}

	result2 := metric2.FormatMetric()
	// With sorted keys, the order should always be: backend, model (alphabetical)
	expected2 := `labeled_metric{backend="llama.cpp",model="test-model"} 123`

	if result2 != expected2 {
		t.Errorf("Expected '%s', got '%s'", expected2, result2)
	}
}

func TestPrometheusParser_parseLabels(t *testing.T) {
	parser := NewPrometheusParser()

	tests := []struct {
		input    string
		expected map[string]string
	}{
		{
			input: `method="get",code="200"`,
			expected: map[string]string{
				"method": "get",
				"code":   "200",
			},
		},
		{
			input: `single="value"`,
			expected: map[string]string{
				"single": "value",
			},
		},
		{
			input:    ``,
			expected: map[string]string{},
		},
	}

	for _, test := range tests {
		result, err := parser.parseLabels(test.input)
		if err != nil {
			t.Errorf("Failed to parse labels '%s': %v", test.input, err)
			continue
		}

		if len(result) != len(test.expected) {
			t.Errorf("For input '%s', expected %d labels, got %d", test.input, len(test.expected), len(result))
			continue
		}

		for key, expectedValue := range test.expected {
			if actualValue, exists := result[key]; !exists {
				t.Errorf("For input '%s', missing expected key '%s'", test.input, key)
			} else if actualValue != expectedValue {
				t.Errorf("For input '%s', key '%s': expected '%s', got '%s'", test.input, key, expectedValue, actualValue)
			}
		}
	}
}
