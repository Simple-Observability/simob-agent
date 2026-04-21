package apache

import (
	"bufio"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"agent/internal/collection"
	"agent/internal/logger"
	"agent/internal/metrics"
)

type ApachePS interface {
	GetStatusPageBody(url string) (string, error)
}

type systemPS struct{}

func (s *systemPS) GetStatusPageBody(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to scrape apache server-status: %w", err)
	}
	defer resp.Body.Close()

	body := new(strings.Builder)
	_, err = bufio.NewReader(resp.Body).WriteTo(body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return body.String(), nil
}

type ApacheCollector struct {
	metrics.BaseCollector

	ps  ApachePS
	url string
}

func NewApacheCollector() *ApacheCollector {
	return &ApacheCollector{
		ps:  &systemPS{},
		url: "http://localhost/server-status?auto",
	}
}

func (c *ApacheCollector) Name() string {
	return "apache"
}

// apacheStats is an internal type used to store the result of the server-status parsing
type apacheStats struct {
	Timestamp             int64
	RequestsTotal         *float64
	RequestsRate          *float64
	BytesTotal            *float64
	BytesPerSecond        *float64
	WorkersBusy           *float64
	WorkersIdle           *float64
	ConnectionsTotal      *float64
	ConnectionsWriting    *float64
	ConnectionsKeepAlive  *float64
	ConnectionsClosing    *float64
}

// apacheMetrics list the available metrics inside the apache package
var apacheMetrics = []struct {
	name   string
	getVal func(current *apacheStats) *float64
}{
	{
		"apache_requests_total",
		func(current *apacheStats) *float64 { return current.RequestsTotal },
	},
	{
		"apache_requests_rate",
		func(current *apacheStats) *float64 { return current.RequestsRate },
	},
	{
		"apache_bytes_total",
		func(current *apacheStats) *float64 { return current.BytesTotal },
	},
	{
		"apache_bytes_bps",
		func(current *apacheStats) *float64 { return current.BytesPerSecond },
	},
	{
		"apache_workers_busy_total",
		func(current *apacheStats) *float64 { return current.WorkersBusy },
	},
	{
		"apache_workers_idle_total",
		func(current *apacheStats) *float64 { return current.WorkersIdle },
	},
	{
		"apache_connections_total",
		func(current *apacheStats) *float64 { return current.ConnectionsTotal },
	},
	{
		"apache_connections_writing_total",
		func(current *apacheStats) *float64 { return current.ConnectionsWriting },
	},
	{
		"apache_connections_keepalive_total",
		func(current *apacheStats) *float64 { return current.ConnectionsKeepAlive },
	},
	{
		"apache_connections_closing_total",
		func(current *apacheStats) *float64 { return current.ConnectionsClosing },
	},
}

func (c *ApacheCollector) Collect() ([]metrics.DataPoint, error) {
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

func (c *ApacheCollector) CollectAll() ([]metrics.DataPoint, error) {
	stats, err := c.getStatsFromStatusPage()
	if err != nil {
		logger.Log.Debug("Failed to collect metrics", "collector", c.Name(), "error", err)
		return nil, nil
	}

	var results []metrics.DataPoint
	for _, m := range apacheMetrics {
		val := m.getVal(stats)
		if val == nil {
			continue
		}
		results = append(results, metrics.DataPoint{
			Name:      m.name,
			Timestamp: stats.Timestamp,
			Value:     *val,
			Labels:    map[string]string{},
		})
	}

	return results, nil
}

func (c *ApacheCollector) Discover() ([]collection.Metric, error) {
	stats, err := c.getStatsFromStatusPage()
	if err != nil {
		return nil, nil
	}

	var discovered []collection.Metric
	for _, m := range apacheMetrics {
		if m.getVal(stats) == nil {
			continue
		}
		discovered = append(discovered, collection.Metric{
			Name:   m.name,
			Type:   "gauge",
			Labels: map[string]string{},
		})
	}
	return discovered, nil
}

func (c *ApacheCollector) getStatsFromStatusPage() (*apacheStats, error) {
	timestamp := time.Now().UnixMilli()
	body, err := c.ps.GetStatusPageBody(c.url)
	if err != nil {
		return nil, fmt.Errorf("failed to get server-status response: %w", err)
	}

	stats, err := parseServerStatus(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse server-status: %w", err)
	}
	stats.Timestamp = timestamp

	return stats, nil
}

func parseServerStatus(body string) (*apacheStats, error) {
	stats := &apacheStats{}
	scanner := bufio.NewScanner(strings.NewReader(body))
	foundKnownField := false

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		key := strings.TrimSpace(parts[0])
		valueStr := strings.TrimSpace(parts[1])

		val, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			continue
		}

		switch key {
		case "Total Accesses":
			stats.RequestsTotal = &val
			foundKnownField = true
		case "Total kBytes":
			bytes := val * 1024
			stats.BytesTotal = &bytes
			foundKnownField = true
		case "ReqPerSec":
			stats.RequestsRate = &val
			foundKnownField = true
		case "BytesPerSec":
			stats.BytesPerSecond = &val
			foundKnownField = true
		case "BusyWorkers":
			stats.WorkersBusy = &val
			foundKnownField = true
		case "IdleWorkers":
			stats.WorkersIdle = &val
			foundKnownField = true
		case "ConnsTotal":
			stats.ConnectionsTotal = &val
			foundKnownField = true
		case "ConnsAsyncWriting":
			stats.ConnectionsWriting = &val
			foundKnownField = true
		case "ConnsAsyncKeepAlive":
			stats.ConnectionsKeepAlive = &val
			foundKnownField = true
		case "ConnsAsyncClosing":
			stats.ConnectionsClosing = &val
			foundKnownField = true
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if !foundKnownField {
		return nil, fmt.Errorf("response did not contain apache server-status fields")
	}

	return stats, nil
}
