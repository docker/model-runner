package metrics

import (
	"bufio"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// PrometheusMetric represents a single Prometheus metric
type PrometheusMetric struct {
	Name   string
	Labels map[string]string
	Value  string
	Help   string
	Type   string
}

// PrometheusParser parses Prometheus text format metrics
type PrometheusParser struct {
	commentRegex *regexp.Regexp
	metricRegex  *regexp.Regexp
}

// NewPrometheusParser creates a new Prometheus metrics parser
func NewPrometheusParser() *PrometheusParser {
	return &PrometheusParser{
		// Matches # HELP and # TYPE comments
		commentRegex: regexp.MustCompile(`^#\s+(HELP|TYPE)\s+(\S+)\s+(.*)$`),
		// Matches metric lines with optional labels
		metricRegex: regexp.MustCompile(`^([a-zA-Z_:][a-zA-Z0-9_:]*?)(\{[^}]*})?\s+(\S+)(\s+\d+)?$`),
	}
}

// ParseMetrics parses Prometheus text format and returns structured metrics
func (p *PrometheusParser) ParseMetrics(content string) ([]PrometheusMetric, error) {
	var metrics []PrometheusMetric
	helpMap := make(map[string]string)
	typeMap := make(map[string]string)

	scanner := bufio.NewScanner(strings.NewReader(content))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines
		if line == "" {
			continue
		}

		// Handle comments (HELP and TYPE)
		if strings.HasPrefix(line, "#") {
			matches := p.commentRegex.FindStringSubmatch(line)
			if len(matches) == 4 {
				directive := matches[1]
				metricName := matches[2]
				content := matches[3]

				switch directive {
				case "HELP":
					helpMap[metricName] = content
				case "TYPE":
					typeMap[metricName] = content
				}
			}
			continue
		}

		// Parse metric lines
		matches := p.metricRegex.FindStringSubmatch(line)
		if len(matches) >= 4 {
			metricName := matches[1]
			labelsStr := matches[2]
			value := matches[3]

			// Parse labels if present
			labels := make(map[string]string)
			if labelsStr != "" {
				// Remove surrounding braces
				labelsStr = strings.Trim(labelsStr, "{}")
				if labelsStr != "" {
					parsedLabels, err := p.parseLabels(labelsStr)
					if err != nil {
						continue // Skip malformed metrics
					}
					labels = parsedLabels
				}
			}

			metric := PrometheusMetric{
				Name:   metricName,
				Labels: labels,
				Value:  value,
				Help:   helpMap[metricName],
				Type:   typeMap[metricName],
			}

			metrics = append(metrics, metric)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning metrics: %w", err)
	}

	return metrics, nil
}

// parseLabels parses label string like 'key1="value1",key2="value2"'
func (p *PrometheusParser) parseLabels(labelsStr string) (map[string]string, error) {
	labels := make(map[string]string)
	// Split by comma, then key="value"
	re := regexp.MustCompile(`(\w+)="([^"]*)"`)

	matches := re.FindAllStringSubmatch(labelsStr, -1)
	for _, match := range matches {
		if len(match) == 3 {
			labels[match[1]] = match[2]
		}
	}
	return labels, nil
}

// AddLabels adds additional labels to a metric
func (m *PrometheusMetric) AddLabels(additionalLabels map[string]string) {
	if m.Labels == nil {
		m.Labels = make(map[string]string)
	}

	for key, value := range additionalLabels {
		m.Labels[key] = value
	}
}

// FormatMetric formats a metric back to Prometheus text format
func (m *PrometheusMetric) FormatMetric() string {
	if len(m.Labels) == 0 {
		return fmt.Sprintf("%s %s", m.Name, m.Value)
	}

	// Sort label keys to ensure consistent output order
	var keys []string
	for key := range m.Labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var labelPairs []string
	for _, key := range keys {
		labelPairs = append(labelPairs, fmt.Sprintf(`%s="%s"`, key, m.Labels[key]))
	}

	return fmt.Sprintf("%s{%s} %s", m.Name, strings.Join(labelPairs, ","), m.Value)
}
