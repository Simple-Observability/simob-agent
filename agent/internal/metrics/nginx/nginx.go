package nginx

import (
	"bufio"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"agent/internal/collection"
	"agent/internal/metrics"
)

type NginxCollector struct {
	metrics.BaseCollector

	url       string
	lastStats *nginxStats
}

func NewNginxCollector() *NginxCollector {
	return &NginxCollector{url: "http://localhost/nginx_status"}
}

func (c *NginxCollector) Name() string {
	return "nginx"
}

// nginxStats is an internal type used to store the result of the stub status parsing
type nginxStats struct {
	Ts       int64
	Active   float64
	Reading  float64
	Writing  float64
	Waiting  float64
	Requests uint64
}

// nginxMetrics list the available metrics inside the nginx package
var nginxMetrics = []struct {
	name   string
	unit   string
	getVal func(current, previous *nginxStats) float64
}{
	{
		"nginx_connections_active_total", "no",
		func(current, previous *nginxStats) float64 { return current.Active },
	},
	{
		"nginx_connections_reading_total", "no",
		func(current, previous *nginxStats) float64 { return current.Reading },
	},
	{
		"nginx_connections_writing_total", "no",
		func(current, previous *nginxStats) float64 { return current.Writing },
	},
	{
		"nginx_connections_waiting_total", "no",
		func(current, previous *nginxStats) float64 { return current.Waiting },
	},
	{
		"nginx_requests_total", "no",
		func(current, previous *nginxStats) float64 { return float64(current.Requests) },
	},
	{
		"nginx_requests_rate", "rate",
		func(current, previous *nginxStats) float64 {
			if previous == nil {
				return 0
			}
			deltaT := float64(current.Ts - previous.Ts)
			var deltaReq float64
			// Counter reset detected (Nginx restart)
			if previous.Requests > current.Requests {
				deltaReq = float64(current.Requests)
			} else {
				deltaReq = float64(current.Requests - previous.Requests)
			}
			return deltaReq / deltaT * 1000
		},
	},
}

func (c *NginxCollector) Collect() ([]metrics.DataPoint, error) {
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

func (c *NginxCollector) CollectAll() ([]metrics.DataPoint, error) {
	stats, err := getStatsFromStatusPage(c.url)
	if err != nil {
		return nil, fmt.Errorf("failed to get stats from nginx status page: %w", err)
	}

	var results []metrics.DataPoint
	for _, m := range nginxMetrics {
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

func (c *NginxCollector) Discover() ([]collection.Metric, error) {
	_, err := getStatsFromStatusPage(c.url)
	if err != nil {
		return nil, nil
	}

	var discovered []collection.Metric
	for _, m := range nginxMetrics {
		discovered = append(discovered, collection.Metric{
			Name:   m.name,
			Type:   "gauge",
			Unit:   m.unit,
			Labels: map[string]string{},
		})
	}
	return discovered, nil
}

func getStatsFromStatusPage(url string) (*nginxStats, error) {
	timestamp := time.Now().UnixMilli()
	body, err := getStatusPageBody(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get stub_status response: %w", err)
	}

	stats, err := parseStubStatus(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse stub_status: %w", err)
	}
	stats.Ts = timestamp

	return stats, err
}

func getStatusPageBody(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to scrape nginx stub_status: %w", err)
	}
	defer resp.Body.Close()

	body := new(strings.Builder)
	_, err = bufio.NewReader(resp.Body).WriteTo(body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return body.String(), nil
}

// parseStubStatus parse the nginx response body and extract values
func parseStubStatus(body string) (*nginxStats, error) {
	stats := &nginxStats{}
	scanner := bufio.NewScanner(strings.NewReader(body))
	lineNo := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineNo++

		if strings.HasPrefix(line, "Active connections:") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				val, err := strconv.ParseFloat(parts[2], 64)
				if err != nil {
					return nil, fmt.Errorf("failed to parse active connections: %w", err)
				}
				stats.Active = val
			}
		} else if lineNo == 3 {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				val, err := strconv.ParseUint(parts[2], 10, 64)
				if err != nil {
					return nil, fmt.Errorf("failed to parse requests count: %w", err)
				}
				stats.Requests = val
			}
		} else if strings.HasPrefix(line, "Reading:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				val, err := strconv.ParseFloat(parts[1], 64)
				if err != nil {
					return nil, fmt.Errorf("failed to parse reading: %w", err)
				}
				stats.Reading = val
			}
			if len(parts) >= 4 {
				val, err := strconv.ParseFloat(parts[3], 64)
				if err != nil {
					return nil, fmt.Errorf("failed to parse writing: %w", err)
				}
				stats.Writing = val
			}
			if len(parts) >= 6 {
				val, err := strconv.ParseFloat(parts[5], 64)
				if err != nil {
					return nil, fmt.Errorf("failed to parse waiting: %w", err)
				}
				stats.Waiting = val
			}
		}
	}

	return stats, scanner.Err()
}
