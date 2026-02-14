// Package autoscaler provides auto-scaling capabilities for dmrlet.
package autoscaler

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/model-runner/pkg/dmrlet/service"
)

// MetricsConfig configures the metrics collector.
type MetricsConfig struct {
	CollectionInterval time.Duration
	WindowSize         time.Duration
}

// DefaultMetricsConfig returns default metrics configuration.
func DefaultMetricsConfig() MetricsConfig {
	return MetricsConfig{
		CollectionInterval: 10 * time.Second,
		WindowSize:         60 * time.Second,
	}
}

// Metrics holds collected metrics for a model.
type Metrics struct {
	Model          string
	QPS            float64   // Requests per second
	LatencyP50     float64   // 50th percentile latency (ms)
	LatencyP99     float64   // 99th percentile latency (ms)
	GPUUtilization []float64 // GPU utilization percentages
	Timestamp      time.Time
}

// Collector collects metrics from inference containers.
type Collector struct {
	mu       sync.RWMutex
	registry *service.Registry
	config   MetricsConfig

	// Rolling window of metrics
	metrics map[string][]Metrics // model -> time series
}

// NewCollector creates a new metrics collector.
func NewCollector(registry *service.Registry, config MetricsConfig) *Collector {
	return &Collector{
		registry: registry,
		config:   config,
		metrics:  make(map[string][]Metrics),
	}
}

// Run starts the metrics collection loop.
func (c *Collector) Run(ctx context.Context) {
	ticker := time.NewTicker(c.config.CollectionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.collectAll(ctx)
			c.pruneOldMetrics()
		}
	}
}

func (c *Collector) collectAll(ctx context.Context) {
	models := c.registry.ListModels()

	for _, model := range models {
		metrics, err := c.collectForModel(ctx, model)
		if err != nil {
			continue
		}

		c.mu.Lock()
		c.metrics[model] = append(c.metrics[model], metrics)
		c.mu.Unlock()
	}
}

func (c *Collector) collectForModel(ctx context.Context, model string) (Metrics, error) {
	entries := c.registry.GetByModel(model)
	if len(entries) == 0 {
		return Metrics{}, fmt.Errorf("no endpoints for model %s", model)
	}

	metrics := Metrics{
		Model:     model,
		Timestamp: time.Now(),
	}

	// Collect QPS and latency from endpoints
	var totalQPS float64
	var latencies []float64
	var gpuUtils []float64

	for _, entry := range entries {
		// Try to get metrics from /metrics endpoint
		endpointMetrics, err := c.fetchEndpointMetrics(ctx, entry.Endpoint)
		if err == nil {
			totalQPS += endpointMetrics.QPS
			if endpointMetrics.LatencyP50 > 0 {
				latencies = append(latencies, endpointMetrics.LatencyP50)
			}
		}

		// Collect GPU utilization for assigned GPUs
		for _, gpuIdx := range entry.GPUs {
			util := c.getGPUUtilization(gpuIdx)
			if util >= 0 {
				gpuUtils = append(gpuUtils, util)
			}
		}
	}

	metrics.QPS = totalQPS
	if len(latencies) > 0 {
		metrics.LatencyP50 = average(latencies)
	}
	metrics.GPUUtilization = gpuUtils

	return metrics, nil
}

type endpointMetrics struct {
	QPS        float64
	LatencyP50 float64
	LatencyP99 float64
}

func (c *Collector) fetchEndpointMetrics(ctx context.Context, endpoint string) (endpointMetrics, error) {
	// Try Prometheus metrics endpoint first
	metricsURL := fmt.Sprintf("http://%s/metrics", endpoint)

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metricsURL, http.NoBody)
	if err != nil {
		return endpointMetrics{}, err
	}

	resp, err := client.Do(req)
	if err != nil {
		// Fallback to stats endpoint
		return c.fetchStatsEndpoint(ctx, endpoint)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.fetchStatsEndpoint(ctx, endpoint)
	}

	// Parse Prometheus format metrics
	return c.parsePrometheusMetrics(resp.Body)
}

func (c *Collector) fetchStatsEndpoint(ctx context.Context, endpoint string) (endpointMetrics, error) {
	// Try vLLM stats endpoint
	statsURL := fmt.Sprintf("http://%s/stats", endpoint)

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, statsURL, http.NoBody)
	if err != nil {
		return endpointMetrics{}, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return endpointMetrics{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return endpointMetrics{}, fmt.Errorf("stats endpoint returned %d", resp.StatusCode)
	}

	var stats map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return endpointMetrics{}, err
	}

	metrics := endpointMetrics{}
	if qps, ok := stats["requests_per_second"].(float64); ok {
		metrics.QPS = qps
	}
	if lat, ok := stats["latency_p50"].(float64); ok {
		metrics.LatencyP50 = lat
	}
	if lat, ok := stats["latency_p99"].(float64); ok {
		metrics.LatencyP99 = lat
	}

	return metrics, nil
}

func (c *Collector) parsePrometheusMetrics(body io.Reader) (endpointMetrics, error) {
	scanner := bufio.NewScanner(body)
	var qps, latencyP50, latencyP99 float64

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip comments and empty lines
		}

		// Split on whitespace to separate metric name and value
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		// Find the last space to separate metric name from value
		lastSpace := strings.LastIndex(line, " ")
		if lastSpace == -1 {
			continue
		}

		metricName := strings.TrimSpace(line[:lastSpace])
		valueStr := strings.TrimSpace(line[lastSpace+1:])

		// Handle timestamp if present (optional third field)
		if strings.Contains(valueStr, " ") {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				valueStr = fields[len(fields)-2] // second to last field
			}
		}

		value, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			continue
		}

		// Common metric names for inference servers
		switch {
		case strings.Contains(metricName, "requests_per_second") ||
			strings.Contains(metricName, "qps") ||
			strings.Contains(metricName, "request_count") ||
			strings.Contains(metricName, "requests_total"):
			qps = value
		case strings.Contains(metricName, "latency_p50") ||
			strings.Contains(metricName, "latency_50") ||
			strings.Contains(metricName, "quantile_0_5"):
			latencyP50 = value
		case strings.Contains(metricName, "latency_p99") ||
			strings.Contains(metricName, "latency_99") ||
			strings.Contains(metricName, "quantile_0_99"):
			latencyP99 = value
		}
	}

	return endpointMetrics{
		QPS:        qps,
		LatencyP50: latencyP50,
		LatencyP99: latencyP99,
	}, nil
}

func (c *Collector) getGPUUtilization(gpuIdx int) float64 {
	// Use nvidia-smi for NVIDIA GPUs
	cmd := exec.Command("nvidia-smi",
		"--query-gpu=utilization.gpu",
		"--format=csv,noheader,nounits",
		fmt.Sprintf("--id=%d", gpuIdx))

	output, err := cmd.Output()
	if err != nil {
		return -1
	}

	util, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		return -1
	}

	return util
}

func (c *Collector) pruneOldMetrics() {
	c.mu.Lock()
	defer c.mu.Unlock()

	cutoff := time.Now().Add(-c.config.WindowSize)

	for model, series := range c.metrics {
		var pruned []Metrics
		for _, m := range series {
			if m.Timestamp.After(cutoff) {
				pruned = append(pruned, m)
			}
		}
		c.metrics[model] = pruned
	}
}

// Get returns the latest metrics for a model.
func (c *Collector) Get(model string) *Metrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	series := c.metrics[model]
	if len(series) == 0 {
		return nil
	}

	latest := series[len(series)-1]
	return &latest
}

// GetAverage returns average metrics over the window for a model.
func (c *Collector) GetAverage(model string) *Metrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	series := c.metrics[model]
	if len(series) == 0 {
		return nil
	}

	var qpsSum, latSum float64
	var gpuUtilSum []float64

	for _, m := range series {
		qpsSum += m.QPS
		latSum += m.LatencyP50
		for i, util := range m.GPUUtilization {
			if len(gpuUtilSum) <= i {
				gpuUtilSum = append(gpuUtilSum, 0)
			}
			gpuUtilSum[i] += util
		}
	}

	n := float64(len(series))
	avgMetrics := &Metrics{
		Model:      model,
		QPS:        qpsSum / n,
		LatencyP50: latSum / n,
		Timestamp:  time.Now(),
	}

	if len(gpuUtilSum) > 0 {
		avgMetrics.GPUUtilization = make([]float64, len(gpuUtilSum))
		for i, sum := range gpuUtilSum {
			avgMetrics.GPUUtilization[i] = sum / n
		}
	}

	return avgMetrics
}

// GetHistory returns the metrics history for a model.
func (c *Collector) GetHistory(model string) []Metrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	series := c.metrics[model]
	result := make([]Metrics, len(series))
	copy(result, series)
	return result
}

func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}
