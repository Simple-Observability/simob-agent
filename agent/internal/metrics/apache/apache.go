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

	ps        ApachePS
	url       string
	lastStats *apacheStats
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
	Ts               int64
	AccessesTotal    float64
	BytesTotal       float64
	UptimeSeconds    float64
	WorkersBusy      float64
	WorkersIdle      float64
	ScboardWaiting   float64
	ScboardStarting  float64
	ScboardReading   float64
	ScboardSending   float64
	ScboardKeepalive float64
	ScboardDnslookup float64
	ScboardClosing   float64
	ScboardLogging   float64
	ScboardFinishing float64
	ScboardIdle      float64
	ScboardOpen      float64
}

// apacheMetrics list the available metrics inside the apache package
var apacheMetrics = []struct {
	name   string
	unit   string
	getVal func(current, previous *apacheStats) float64
}{
	{
		"apache_accesses_total", "no",
		func(current, previous *apacheStats) float64 { return current.AccessesTotal },
	},
	{
		"apache_requests_rate", "rate",
		func(current, previous *apacheStats) float64 {
			if previous == nil {
				return 0
			}
			deltaT := float64(current.Ts - previous.Ts)
			if deltaT == 0 {
				return 0
			}
			var deltaReq float64
			if previous.AccessesTotal > current.AccessesTotal {
				deltaReq = float64(current.AccessesTotal)
			} else {
				deltaReq = float64(current.AccessesTotal - previous.AccessesTotal)
			}
			return deltaReq / deltaT * 1000
		},
	},
	{
		"apache_bytes_total", "bytes",
		func(current, previous *apacheStats) float64 { return current.BytesTotal },
	},
	{
		"apache_bytes_rate", "rate",
		func(current, previous *apacheStats) float64 {
			if previous == nil {
				return 0
			}
			deltaT := float64(current.Ts - previous.Ts)
			if deltaT == 0 {
				return 0
			}
			var deltaBytes float64
			if previous.BytesTotal > current.BytesTotal {
				deltaBytes = float64(current.BytesTotal)
			} else {
				deltaBytes = float64(current.BytesTotal - previous.BytesTotal)
			}
			return deltaBytes / deltaT * 1000
		},
	},
	{
		"apache_uptime_seconds", "s",
		func(current, previous *apacheStats) float64 { return current.UptimeSeconds },
	},
	{
		"apache_workers_busy_total", "no",
		func(current, previous *apacheStats) float64 { return current.WorkersBusy },
	},
	{
		"apache_workers_idle_total", "no",
		func(current, previous *apacheStats) float64 { return current.WorkersIdle },
	},
	{
		"apache_scoreboard_waiting_total", "no",
		func(current, previous *apacheStats) float64 { return current.ScboardWaiting },
	},
	{
		"apache_scoreboard_starting_total", "no",
		func(current, previous *apacheStats) float64 { return current.ScboardStarting },
	},
	{
		"apache_scoreboard_reading_total", "no",
		func(current, previous *apacheStats) float64 { return current.ScboardReading },
	},
	{
		"apache_scoreboard_sending_total", "no",
		func(current, previous *apacheStats) float64 { return current.ScboardSending },
	},
	{
		"apache_scoreboard_keepalive_total", "no",
		func(current, previous *apacheStats) float64 { return current.ScboardKeepalive },
	},
	{
		"apache_scoreboard_dnslookup_total", "no",
		func(current, previous *apacheStats) float64 { return current.ScboardDnslookup },
	},
	{
		"apache_scoreboard_closing_total", "no",
		func(current, previous *apacheStats) float64 { return current.ScboardClosing },
	},
	{
		"apache_scoreboard_logging_total", "no",
		func(current, previous *apacheStats) float64 { return current.ScboardLogging },
	},
	{
		"apache_scoreboard_finishing_total", "no",
		func(current, previous *apacheStats) float64 { return current.ScboardFinishing },
	},
	{
		"apache_scoreboard_idle_cleanup_total", "no",
		func(current, previous *apacheStats) float64 { return current.ScboardIdle },
	},
	{
		"apache_scoreboard_open_total", "no",
		func(current, previous *apacheStats) float64 { return current.ScboardOpen },
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

func (c *ApacheCollector) Discover() ([]collection.Metric, error) {
	_, err := c.getStatsFromStatusPage()
	if err != nil {
		return nil, nil
	}

	var discovered []collection.Metric
	for _, m := range apacheMetrics {
		discovered = append(discovered, collection.Metric{
			Name:   m.name,
			Type:   "gauge",
			Unit:   m.unit,
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
	stats.Ts = timestamp

	return stats, nil
}

func parseServerStatus(body string) (*apacheStats, error) {
	stats := &apacheStats{}
	scanner := bufio.NewScanner(strings.NewReader(body))

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		key := strings.TrimSpace(parts[0])
		valueStr := strings.TrimSpace(parts[1])

		if key == "Scoreboard" {
			parseScoreboard(valueStr, stats)
			continue
		}

		val, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			continue
		}

		switch key {
		case "Total Accesses":
			stats.AccessesTotal = val
		case "Total kBytes":
			stats.BytesTotal = val * 1024
		case "Uptime":
			stats.UptimeSeconds = val
		case "BusyWorkers":
			stats.WorkersBusy = val
		case "IdleWorkers":
			stats.WorkersIdle = val
		}
	}

	return stats, scanner.Err()
}

func parseScoreboard(data string, stats *apacheStats) {
	for _, char := range data {
		switch char {
		case '_':
			stats.ScboardWaiting++
		case 'S':
			stats.ScboardStarting++
		case 'R':
			stats.ScboardReading++
		case 'W':
			stats.ScboardSending++
		case 'K':
			stats.ScboardKeepalive++
		case 'D':
			stats.ScboardDnslookup++
		case 'C':
			stats.ScboardClosing++
		case 'L':
			stats.ScboardLogging++
		case 'G':
			stats.ScboardFinishing++
		case 'I':
			stats.ScboardIdle++
		case '.':
			stats.ScboardOpen++
		}
	}
}
