package caddy

import (
	"fmt"
	"net/http"
	"time"

	"agent/internal/collection"
	"agent/internal/logger"
	"agent/internal/metrics"

	dto "github.com/prometheus/client_model/go"
)

type CaddyPS interface {
	GetMetrics(url string) (map[string]*dto.MetricFamily, error)
}

type systemPS struct{}

func (s *systemPS) GetMetrics(url string) (map[string]*dto.MetricFamily, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to scrape caddy metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("caddy metrics returned status: %s", resp.Status)
	}

	return metrics.ParsePrometheus(resp.Body)
}

type caddyStats struct {
	Ts              int64
	Requests        float64
	Errors          float64
	ResponseBytes   float64
	TotalDurationMs float64
	MemoryBytes     float64
	CPUTotal        float64
}

type CaddyCollector struct {
	metrics.BaseCollector

	ps        CaddyPS
	url       string
	lastStats *caddyStats
}

func NewCaddyCollector() *CaddyCollector {
	return &CaddyCollector{
		ps:  &systemPS{},
		url: "http://localhost:2019/metrics",
	}
}

func (c *CaddyCollector) Name() string {
	return "caddy"
}

func getRate(getter func(*caddyStats) float64) func(current, previous *caddyStats) float64 {
	return func(current, previous *caddyStats) float64 {
		if previous == nil {
			return 0
		}
		deltaT := float64(current.Ts-previous.Ts) / 1000.0
		if deltaT <= 0 {
			return 0
		}
		val := getter(current)
		prevVal := getter(previous)
		delta := val - prevVal
		if val < prevVal {
			// Counter reset detected
			delta = val
		}
		return delta / deltaT
	}
}

func getGauge(getter func(*caddyStats) float64) func(current, previous *caddyStats) float64 {
	return func(current, previous *caddyStats) float64 {
		return getter(current)
	}
}

var caddyMetrics = []struct {
	name   string
	getVal func(current, previous *caddyStats) float64
}{
	{"caddy_http_requests_total", getGauge(func(s *caddyStats) float64 { return s.Requests })},
	{"caddy_http_requests_rate", getRate(func(s *caddyStats) float64 { return s.Requests })},
	{"caddy_http_errors_total", getGauge(func(s *caddyStats) float64 { return s.Errors })},
	{"caddy_http_errors_rate", getRate(func(s *caddyStats) float64 { return s.Errors })},
	{"caddy_http_response_bytes", getGauge(func(s *caddyStats) float64 { return s.ResponseBytes })},
	{"caddy_http_response_bps", getRate(func(s *caddyStats) float64 { return s.ResponseBytes })},
	{"caddy_http_request_duration_ms", getGauge(func(s *caddyStats) float64 { return s.TotalDurationMs })},
	{"caddy_memory_bytes", getGauge(func(s *caddyStats) float64 { return s.MemoryBytes })},
	{"caddy_cpu_total", getGauge(func(s *caddyStats) float64 { return s.CPUTotal })},
	{"caddy_cpu_rate", getRate(func(s *caddyStats) float64 { return s.CPUTotal })},
}

func (c *CaddyCollector) Collect() ([]metrics.DataPoint, error) {
	all, err := c.CollectAll()
	if err != nil {
		return nil, err
	}
	var included []metrics.DataPoint
	for _, dp := range all {
		if c.IsIncluded(dp.Name, dp.Labels) {
			included = append(included, dp)
		}
	}
	return included, nil
}

func (c *CaddyCollector) CollectAll() ([]metrics.DataPoint, error) {
	stats, err := c.getStats()
	if err != nil {
		logger.Log.Debug("Failed to collect metrics", "collector", c.Name(), "error", err)
		return nil, nil
	}

	var results []metrics.DataPoint
	for _, m := range caddyMetrics {
		val := m.getVal(stats, c.lastStats)
		results = append(results, metrics.DataPoint{
			Name:      m.name,
			Timestamp: stats.Ts,
			Value:     val,
			Labels:    map[string]string{},
		})
	}

	c.lastStats = stats

	return results, nil
}

func (c *CaddyCollector) Discover() ([]collection.Metric, error) {
	_, err := c.getStats()
	if err != nil {
		return nil, nil
	}

	var discovered []collection.Metric
	for _, m := range caddyMetrics {
		discovered = append(discovered, collection.Metric{
			Name:   m.name,
			Type:   "gauge",
			Labels: map[string]string{},
		})
	}
	return discovered, nil
}

func (c *CaddyCollector) getStats() (*caddyStats, error) {
	timestamp := time.Now().UnixMilli()
	families, err := c.ps.GetMetrics(c.url)
	if err != nil {
		return nil, err
	}

	stats := &caddyStats{
		Ts: timestamp,
	}

	for name, family := range families {
		switch name {
		case "caddy_http_requests_total":
			stats.Requests = sumFamily(family)
		case "caddy_http_request_errors_total":
			stats.Errors = sumFamily(family)
		case "caddy_http_response_size_bytes":
			stats.ResponseBytes = sumFamily(family)
		case "caddy_http_request_duration_seconds":
			stats.TotalDurationMs = sumFamily(family) * 1000
		case "process_resident_memory_bytes":
			stats.MemoryBytes = sumFamily(family)
		case "process_cpu_seconds_total":
			stats.CPUTotal = sumFamily(family)
		}
	}

	return stats, nil
}

func sumFamily(family *dto.MetricFamily) float64 {
	var sum float64
	for _, m := range family.Metric {
		sum += metrics.GetMetricValue(m)
	}
	return sum
}
