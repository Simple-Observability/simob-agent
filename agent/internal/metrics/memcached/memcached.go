package memcached

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"agent/internal/collection"
	"agent/internal/logger"
	"agent/internal/metrics"
)

type MemcachedPS interface {
	GetStats(address string) (string, error)
}

type systemPS struct {
	timeout time.Duration
}

func (s *systemPS) GetStats(address string) (string, error) {
	conn, err := net.DialTimeout("tcp", address, s.timeout)
	if err != nil {
		return "", fmt.Errorf("failed to connect to memcached: %w", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(s.timeout)); err != nil {
		return "", fmt.Errorf("failed to set deadline: %w", err)
	}

	fmt.Fprint(conn, "stats\r\n")

	var stats strings.Builder
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "END" {
			break
		}
		stats.WriteString(line)
		stats.WriteString("\n")
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read from memcached: %w", err)
	}

	return stats.String(), nil
}

type MemcachedCollector struct {
	metrics.BaseCollector

	ps        MemcachedPS
	address   string
	lastStats *memcachedStats
}

func NewMemcachedCollector() *MemcachedCollector {
	return &MemcachedCollector{
		ps:      &systemPS{timeout: 5 * time.Second},
		address: "127.0.0.1:11211",
	}
}

func (c *MemcachedCollector) Name() string {
	return "memcached"
}

type memcachedStats struct {
	Ts    int64
	Stats map[string]float64
}

func getRate(key string) func(current, previous *memcachedStats) float64 {
	return func(current, previous *memcachedStats) float64 {
		if previous == nil {
			return 0
		}
		deltaT := float64(current.Ts-previous.Ts) / 1000.0
		if deltaT <= 0 {
			return 0
		}
		val := current.Stats[key]
		prevVal := previous.Stats[key]
		delta := val - prevVal
		if val < prevVal {
			// Counter reset detected
			delta = val
		}
		return delta / deltaT
	}
}

func getGauge(key string) func(current, previous *memcachedStats) float64 {
	return func(current, previous *memcachedStats) float64 {
		return current.Stats[key]
	}
}

var memcachedMetrics = []struct {
	name   string
	unit   string
	getVal func(current, previous *memcachedStats) float64
}{
	{"memcached_uptime_seconds", "s", getGauge("uptime")},
	{"memcached_connections_current_total", "no", getGauge("curr_connections")},
	{"memcached_connections_rate", "rate", getRate("total_connections")},
	{"memcached_items_current_total", "no", getGauge("curr_items")},
	{"memcached_items_rate", "rate", getRate("total_items")},
	{"memcached_get_rate", "rate", getRate("cmd_get")},
	{"memcached_set_rate", "rate", getRate("cmd_set")},
	{"memcached_get_hits_rate", "rate", getRate("get_hits")},
	{"memcached_get_misses_rate", "rate", getRate("get_misses")},
	{"memcached_delete_hits_rate", "rate", getRate("delete_hits")},
	{"memcached_delete_misses_rate", "rate", getRate("delete_misses")},
	{"memcached_incr_hits_rate", "rate", getRate("incr_hits")},
	{"memcached_incr_misses_rate", "rate", getRate("incr_misses")},
	{"memcached_decr_hits_rate", "rate", getRate("decr_hits")},
	{"memcached_decr_misses_rate", "rate", getRate("decr_misses")},
	{"memcached_read_bps", "bps", getRate("bytes_read")},
	{"memcached_written_bps", "bps", getRate("bytes_written")},
	{"memcached_limit_bytes", "bytes", getGauge("limit_maxbytes")},
	{"memcached_used_bytes", "bytes", getGauge("bytes")},
}

func (c *MemcachedCollector) Collect() ([]metrics.DataPoint, error) {
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

func (c *MemcachedCollector) CollectAll() ([]metrics.DataPoint, error) {
	stats, err := c.getStats()
	if err != nil {
		logger.Log.Debug("Failed to collect metrics", "collector", c.Name(), "error", err)
		return nil, nil
	}

	var results []metrics.DataPoint
	for _, m := range memcachedMetrics {
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

func (c *MemcachedCollector) Discover() ([]collection.Metric, error) {
	_, err := c.ps.GetStats(c.address)
	if err != nil {
		return nil, nil
	}

	var discovered []collection.Metric
	for _, m := range memcachedMetrics {
		discovered = append(discovered, collection.Metric{
			Name:   m.name,
			Type:   "gauge",
			Unit:   m.unit,
			Labels: map[string]string{},
		})
	}
	return discovered, nil
}

func (c *MemcachedCollector) getStats() (*memcachedStats, error) {
	timestamp := time.Now().UnixMilli()
	body, err := c.ps.GetStats(c.address)
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	statsMap := parseMemcachedStats(body)
	return &memcachedStats{
		Ts:    timestamp,
		Stats: statsMap,
	}, nil
}

func parseMemcachedStats(body string) map[string]float64 {
	stats := make(map[string]float64)
	scanner := bufio.NewScanner(strings.NewReader(body))

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "STAT" {
			key := fields[1]
			val, err := strconv.ParseFloat(fields[2], 64)
			if err == nil {
				stats[key] = val
			}
		}
	}

	return stats
}
