package metrics

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/docker/model-runner/pkg/logging"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

// AggregatedMetricsHandler collects metrics from all active runners and aggregates them with labels
type AggregatedMetricsHandler struct {
	log       logging.Logger
	scheduler SchedulerInterface
}

// NewAggregatedMetricsHandler creates a new aggregated metrics handler
func NewAggregatedMetricsHandler(log logging.Logger, scheduler SchedulerInterface) *AggregatedMetricsHandler {
	return &AggregatedMetricsHandler{
		log:       log,
		scheduler: scheduler,
	}
}

// ServeHTTP implements http.Handler for aggregated metrics
func (h *AggregatedMetricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	runners := h.scheduler.GetAllActiveRunners()
	if len(runners) == 0 {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "# No active runners\n")
		return
	}

	// Collect and aggregate metrics from all runners
	allFamilies := h.collectAndAggregateMetrics(r.Context(), runners)

	// Write aggregated response using Prometheus encoder
	h.writeAggregatedMetrics(w, allFamilies)
}

// collectAndAggregateMetrics fetches metrics from all runners and aggregates them
func (h *AggregatedMetricsHandler) collectAndAggregateMetrics(ctx context.Context, runners []ActiveRunner) map[string]*dto.MetricFamily {
	var wg sync.WaitGroup
	var mu sync.Mutex
	allFamilies := make(map[string]*dto.MetricFamily)

	for _, runner := range runners {
		wg.Add(1)
		go func(runner ActiveRunner) {
			defer wg.Done()

			families, err := h.fetchRunnerMetrics(ctx, runner)
			if err != nil {
				h.log.Warnf("Failed to fetch metrics from runner %s/%s: %v", runner.BackendName, runner.ModelName, err)
				return
			}

			// Add labels to metrics and merge into allFamilies
			labels := map[string]string{
				"backend": runner.BackendName,
				"model":   runner.ModelName,
				"mode":    runner.Mode,
			}

			mu.Lock()
			h.addLabelsAndMerge(families, labels, allFamilies)
			mu.Unlock()
		}(runner)
	}

	wg.Wait()
	return allFamilies
}

// fetchRunnerMetrics fetches and parses metrics from a single runner
func (h *AggregatedMetricsHandler) fetchRunnerMetrics(ctx context.Context, runner ActiveRunner) (map[string]*dto.MetricFamily, error) {
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
		return nil, fmt.Errorf("failed to create metrics request: %w", err)
	}

	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metrics endpoint returned status %d", resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read metrics response: %w", err)
	}

	// Parse metrics using official Prometheus parser
	parser := expfmt.TextParser{}
	families, err := parser.TextToMetricFamilies(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("failed to parse metrics: %w", err)
	}

	return families, nil
}

// addLabelsAndMerge adds labels to metrics and merges them into the aggregated families
func (h *AggregatedMetricsHandler) addLabelsAndMerge(families map[string]*dto.MetricFamily, labels map[string]string, allFamilies map[string]*dto.MetricFamily) {
	for name, family := range families {
		// Add labels to each metric in the family
		for _, metric := range family.GetMetric() {
			// Add our labels to the existing label pairs
			for key, value := range labels {
				metric.Label = append(metric.Label, &dto.LabelPair{
					Name:  &key,
					Value: &value,
				})
			}
		}

		// Merge into allFamilies
		if existingFamily, exists := allFamilies[name]; exists {
			// Append metrics to existing family
			existingFamily.Metric = append(existingFamily.Metric, family.GetMetric()...)
		} else {
			// Create new family
			allFamilies[name] = family
		}
	}
}

// writeAggregatedMetrics writes the aggregated metrics using Prometheus encoder
func (h *AggregatedMetricsHandler) writeAggregatedMetrics(w http.ResponseWriter, families map[string]*dto.MetricFamily) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	// Use Prometheus encoder to write metrics
	encoder := expfmt.NewEncoder(w, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, family := range families {
		if err := encoder.Encode(family); err != nil {
			h.log.Errorf("Failed to encode metric family %s: %v", *family.Name, err)
			continue
		}
	}
}
