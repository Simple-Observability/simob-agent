package network

import (
	"agent/internal/collection"
	"agent/internal/metrics"
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v4/net"
)

type NetworkPS interface {
	IOCounters(pernic bool) ([]net.IOCountersStat, error)
}

type systemPS struct{}

func (s *systemPS) IOCounters(pernic bool) ([]net.IOCountersStat, error) {
	return net.IOCounters(pernic)
}

type NetworkCollector struct {
	metrics.BaseCollector

	ps        NetworkPS
	lastStats map[string]net.IOCountersStat
	lastTime  time.Time
}

func NewNetworkCollector() *NetworkCollector {
	return &NetworkCollector{
		ps: &systemPS{},
	}
}

func (c *NetworkCollector) Name() string {
	return "net"
}

// netMetrics list the available metrics inside the network package
var netMetrics = []struct {
	name       string
	getCounter func(*net.IOCountersStat) float64
}{
	{"net_bytes_sent_bps", func(io *net.IOCountersStat) float64 { return float64(io.BytesSent) }},
	{"net_bytes_recv_bps", func(io *net.IOCountersStat) float64 { return float64(io.BytesRecv) }},
	{"net_packets_sent_rate", func(io *net.IOCountersStat) float64 { return float64(io.PacketsSent) }},
	{"net_packets_recv_rate", func(io *net.IOCountersStat) float64 { return float64(io.PacketsRecv) }},
	{"net_errin_rate", func(io *net.IOCountersStat) float64 { return float64(io.Errin) }},
	{"net_errout_rate", func(io *net.IOCountersStat) float64 { return float64(io.Errout) }},
	{"net_dropin_rate", func(io *net.IOCountersStat) float64 { return float64(io.Dropin) }},
	{"net_dropout_rate", func(io *net.IOCountersStat) float64 { return float64(io.Dropout) }},
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
	ioStats, err := c.ps.IOCounters(true)
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
	ioStats, err := c.ps.IOCounters(true)
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
				Labels: labels,
			})
		}
	}
	return discovered, nil
}
