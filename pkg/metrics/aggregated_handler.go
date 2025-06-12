package metrics

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/docker/model-runner/pkg/logging"
)

// AggregatedMetricsHandler collects metrics from all active runners and aggregates them with labels
type AggregatedMetricsHandler struct {
	log       logging.Logger
	scheduler SchedulerInterface
	parser    *PrometheusParser
}

// NewAggregatedMetricsHandler creates a new aggregated metrics handler
func NewAggregatedMetricsHandler(log logging.Logger, scheduler SchedulerInterface) *AggregatedMetricsHandler {
	return &AggregatedMetricsHandler{
		log:       log,
		scheduler: scheduler,
		parser:    NewPrometheusParser(),
	}
}

// ServeHTTP implements http.Handler for aggregated metrics
func (h *AggregatedMetricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	runners := h.scheduler.GetAllActiveRunners()
	if len(runners) == 0 {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "# No active runners\n")
		return
	}

	// Collect metrics from all runners concurrently
	allMetrics, helpMap, typeMap := h.collectMetricsFromRunners(r.Context(), runners)

	// Generate aggregated response
	h.writeAggregatedMetrics(w, allMetrics, helpMap, typeMap)
}

// collectMetricsFromRunners fetches metrics from all runners concurrently
func (h *AggregatedMetricsHandler) collectMetricsFromRunners(ctx context.Context, runners []ActiveRunner) ([]PrometheusMetric, map[string]string, map[string]string) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var allMetrics []PrometheusMetric
	helpMap := make(map[string]string)
	typeMap := make(map[string]string)

	for _, runner := range runners {
		wg.Add(1)
		go func(runner ActiveRunner) {
			defer wg.Done()

			metrics, help, types, err := h.fetchRunnerMetrics(ctx, runner)
			if err != nil {
				h.log.Warnf("Failed to fetch metrics from runner %s/%s: %v", runner.BackendName, runner.ModelName, err)
				return
			}

			mu.Lock()
			allMetrics = append(allMetrics, metrics...)
			// Merge help and type maps
			for k, v := range help {
				helpMap[k] = v
			}
			for k, v := range types {
				typeMap[k] = v
			}
			mu.Unlock()
		}(runner)
	}

	wg.Wait()
	return allMetrics, helpMap, typeMap
}

// fetchRunnerMetrics fetches metrics from a single runner
func (h *AggregatedMetricsHandler) fetchRunnerMetrics(ctx context.Context, runner ActiveRunner) ([]PrometheusMetric, map[string]string, map[string]string, error) {
	// Create HTTP client for Unix socket communication
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.DialTimeout("unix", runner.Socket, 5*time.Second)
			},
		},
		Timeout: 10 * time.Second,
	}

	// Create request to the runner's metrics endpoint
	req, err := http.NewRequestWithContext(ctx, "GET", "http://unix/metrics", nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create metrics request: %w", err)
	}

	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to fetch metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, nil, fmt.Errorf("metrics endpoint returned status %d", resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read metrics response: %w", err)
	}

	// Parse metrics
	metrics, err := h.parser.ParseMetrics(string(body))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse metrics: %w", err)
	}

	// Add labels to each metric
	labels := map[string]string{
		"backend": runner.BackendName,
		"model":   runner.ModelName,
		"mode":    runner.Mode,
	}

	for i := range metrics {
		metrics[i].AddLabels(labels)
	}

	// Extract help and type information
	helpMap := make(map[string]string)
	typeMap := make(map[string]string)
	for _, metric := range metrics {
		if metric.Help != "" {
			helpMap[metric.Name] = metric.Help
		}
		if metric.Type != "" {
			typeMap[metric.Name] = metric.Type
		}
	}

	return metrics, helpMap, typeMap, nil
}

// writeAggregatedMetrics writes the aggregated metrics response
func (h *AggregatedMetricsHandler) writeAggregatedMetrics(w http.ResponseWriter, metrics []PrometheusMetric, helpMap map[string]string, typeMap map[string]string) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	// Group metrics by name for better organization
	metricGroups := make(map[string][]PrometheusMetric)
	for _, metric := range metrics {
		metricGroups[metric.Name] = append(metricGroups[metric.Name], metric)
	}

	// Sort metric names for consistent output
	var metricNames []string
	for name := range metricGroups {
		metricNames = append(metricNames, name)
	}
	sort.Strings(metricNames)

	// Write metrics grouped by name
	for _, name := range metricNames {
		group := metricGroups[name]

		// Write HELP comment if available
		if help, exists := helpMap[name]; exists {
			fmt.Fprintf(w, "# HELP %s %s\n", name, help)
		}

		// Write TYPE comment if available
		if metricType, exists := typeMap[name]; exists {
			fmt.Fprintf(w, "# TYPE %s %s\n", name, metricType)
		}

		// Write all metrics with this name
		for _, metric := range group {
			fmt.Fprintf(w, "%s\n", metric.FormatMetric())
		}

		// Add blank line between metric groups for readability
		fmt.Fprintf(w, "\n")
	}

	h.log.Debugf("Successfully served aggregated metrics for %d metric groups", len(metricGroups))
}
