package network

import (
	"agent/internal/metrics"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/net"
)

type NetworkCollector struct {
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
	name     string
	unit     string
	getValue func(*net.IOCountersStat) float64
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
	timestamp := time.Now()
	ioStats, err := net.IOCounters(true)
	if err != nil {
		return nil, fmt.Errorf("failed to collect network IO stats: %w", err)
	}

	if c.lastStats == nil {
		c.lastStats = make(map[string]net.IOCountersStat)
		for _, s := range ioStats {
			if isValidInterface(s.Name) {
				c.lastStats[s.Name] = s
			}
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
		if !isValidInterface(s.Name) {
			continue
		}
		prev, ok := c.lastStats[s.Name]
		if !ok {
			continue
		}
		labels := map[string]string{"interface": s.Name}
		for _, m := range netMetrics {
			delta := m.getValue(&s) - m.getValue(&prev)
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

func (c *NetworkCollector) Discover() ([]metrics.Metric, error) {
	ioStats, err := net.IOCounters(true)
	if err != nil {
		return nil, fmt.Errorf("failed to discover network interfaces: %w", err)
	}

	var discovered []metrics.Metric
	for _, s := range ioStats {
		if !isValidInterface(s.Name) {
			continue
		}
		labels := map[string]string{"interface": s.Name}
		for _, m := range netMetrics {
			discovered = append(discovered, metrics.Metric{
				Name:   m.name,
				Type:   "gauge",
				Unit:   m.unit,
				Labels: labels,
			})
		}
	}
	return discovered, nil
}

func isValidInterface(name string) bool {
	if runtime.GOOS == "darwin" {
		ignored := []string{
			"lo",         // Loopback (localhost)
			"gif", "stf", //	IPv6/IPv4 tunneling
			"anpi", "ap", "awdl", // Access point and Apple wireless direct link
			"llw",    // Low-latency WLAN
			"utun",   // Tunneling (VPNn etc)
			"vmenet", // VM
			"bridge",
		}
		for _, p := range ignored {
			if strings.HasPrefix(name, p) {
				return false
			}
		}
	}
	if runtime.GOOS == "linux" {
		ignored := []string{
			"lo",     // Loopback (localhost)
			"docker", // docker0, default Docker bridge
			"veth",   // Virtual ethernet pair
			"br-",    // Custom Docker bridge networks
			"vmnet",  // VM
			"virbr",  // VM
		}
		for _, p := range ignored {
			if strings.HasPrefix(name, p) {
				return false
			}
		}
	}
	return true
}
