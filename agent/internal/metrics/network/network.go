package network

import (
	"agent/internal/collection"
	"agent/internal/metrics"
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v4/net"
)

type NetworkCollector struct {
	metrics.BaseCollector

	lastStats map[string]net.IOCountersStat
	lastTime  time.Time
}

func NewNetworkCollector() *NetworkCollector {
	return &NetworkCollector{}
}

func (c *NetworkCollector) Name() string {
	return "net"
}

// netMetrics list the available metrics inside the network package
var netMetrics = []struct {
	name       string
	unit       string
	getCounter func(*net.IOCountersStat) float64
}{
	{"net_bytes_sent_bps", "bps", func(io *net.IOCountersStat) float64 { return float64(io.BytesSent) }},
	{"net_bytes_recv_bps", "bps", func(io *net.IOCountersStat) float64 { return float64(io.BytesRecv) }},
	{"net_packets_sent_rate", "rate", func(io *net.IOCountersStat) float64 { return float64(io.PacketsSent) }},
	{"net_packets_recv_rate", "rate", func(io *net.IOCountersStat) float64 { return float64(io.PacketsRecv) }},
	{"net_errin_rate", "rate", func(io *net.IOCountersStat) float64 { return float64(io.Errin) }},
	{"net_errout_rate", "rate", func(io *net.IOCountersStat) float64 { return float64(io.Errout) }},
	{"net_dropin_rate", "rate", func(io *net.IOCountersStat) float64 { return float64(io.Dropin) }},
	{"net_dropout_rate", "rate", func(io *net.IOCountersStat) float64 { return float64(io.Dropout) }},
}

func (c *NetworkCollector) Collect() ([]metrics.DataPoint, error) {
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

func (c *NetworkCollector) CollectAll() ([]metrics.DataPoint, error) {
	timestamp := time.Now()
	ioStats, err := net.IOCounters(true)
	if err != nil {
		return nil, fmt.Errorf("failed to collect network IO stats: %w", err)
	}

	if c.lastStats == nil {
		c.lastStats = make(map[string]net.IOCountersStat)
		for _, s := range ioStats {
			c.lastStats[s.Name] = s
		}
		c.lastTime = timestamp
		return []metrics.DataPoint{}, nil
	}

	deltaT := timestamp.Sub(c.lastTime).Seconds()
	if deltaT <= 0 {
		return nil, nil
	}

	var results []metrics.DataPoint
	for _, s := range ioStats {
		prev, ok := c.lastStats[s.Name]
		if !ok {
			continue
		}
		labels := map[string]string{"interface": s.Name}
		for _, m := range netMetrics {
			delta := m.getCounter(&s) - m.getCounter(&prev)
			value := delta / deltaT
			results = append(results, metrics.DataPoint{
				Name:      m.name,
				Timestamp: timestamp.UnixMilli(),
				Value:     value,
				Labels:    labels,
			})
		}
		c.lastStats[s.Name] = s
	}
	c.lastTime = timestamp
	return results, nil
}

func (c *NetworkCollector) Discover() ([]collection.Metric, error) {
	ioStats, err := net.IOCounters(true)
	if err != nil {
		return nil, fmt.Errorf("failed to discover network interfaces: %w", err)
	}

	var discovered []collection.Metric
	for _, s := range ioStats {
		labels := map[string]string{"interface": s.Name}
		for _, m := range netMetrics {
			discovered = append(discovered, collection.Metric{
				Name:   m.name,
				Type:   "gauge",
				Unit:   m.unit,
				Labels: labels,
			})
		}
	}
	return discovered, nil
}
