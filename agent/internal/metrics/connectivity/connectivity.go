package connectivity

import (
	"net"
	"sync"
	"time"

	"agent/internal/collection"
	"agent/internal/logger"
	"agent/internal/metrics"
)

// Dialer abstracts outbound TCP connections for testability.
type Dialer interface {
	DialTimeout(address string, timeout time.Duration) (time.Duration, error)
}

type systemDialer struct{}

func (s *systemDialer) DialTimeout(address string, timeout time.Duration) (time.Duration, error) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return 0, err
	}
	elapsed := time.Since(start)
	if err := conn.Close(); err != nil {
		return 0, err
	}
	return elapsed, nil
}

// The collector is considered online if at least one target is reachable, which avoids false
// negatives caused by a single provider outage.
var defaultTargets = []string{
	"1.1.1.1:443", // Cloudflare
	"8.8.8.8:443", // Google
	"9.9.9.9:443", // Quad9
}

const dialTimeout = 5 * time.Second

type ConnectivityCollector struct {
	metrics.BaseCollector

	dialer  Dialer
	targets []string
}

func NewConnectivityCollector() *ConnectivityCollector {
	return &ConnectivityCollector{
		dialer:  &systemDialer{},
		targets: defaultTargets,
	}
}

func (c *ConnectivityCollector) Name() string {
	return "connectivity"
}

func (c *ConnectivityCollector) Collect() ([]metrics.DataPoint, error) {
	all, err := c.CollectAll()
	if err != nil {
		return nil, err
	}
	var included []metrics.DataPoint
	for _, dp := range all {
		// The connectivity status signal is always emitted;
		// the latency metric is opt-in via the collection config.
		if dp.Name == "connectivity" || c.IsIncluded(dp.Name, dp.Labels) {
			included = append(included, dp)
		}
	}
	return included, nil
}

func (c *ConnectivityCollector) CollectAll() ([]metrics.DataPoint, error) {
	timestamp := time.Now().UnixMilli()

	type probeResult struct {
		target  string
		latency time.Duration
		err     error
	}
	probes := make([]probeResult, len(c.targets))

	var wg sync.WaitGroup
	for i, target := range c.targets {
		wg.Add(1)
		go func(i int, target string) {
			defer wg.Done()
			latency, err := c.dialer.DialTimeout(target, dialTimeout)
			probes[i] = probeResult{target: target, latency: latency, err: err}
		}(i, target)
	}
	wg.Wait()

	var up float64
	var minLatency = -1.0
	for _, r := range probes {
		if r.err != nil {
			logger.Log.Debug("Connectivity probe failed", "collector", c.Name(), "target", r.target, "error", r.err)
			continue
		}
		up = 1
		latencyMs := float64(r.latency.Microseconds()) / 1000.0
		if minLatency < 0 || latencyMs < minLatency {
			minLatency = latencyMs
		}
	}

	results := []metrics.DataPoint{
		{
			Name:      "connectivity",
			Timestamp: timestamp,
			Value:     up,
			Labels:    map[string]string{},
		},
	}
	if minLatency >= 0 {
		results = append(results, metrics.DataPoint{
			Name:      "connectivity_latency_ms",
			Timestamp: timestamp,
			Value:     minLatency,
			Labels:    map[string]string{},
		})
	}
	return results, nil
}

func (c *ConnectivityCollector) Discover() ([]collection.Metric, error) {
	// Only the latency metric is advertised: it is selectable in the
	// collection config. The `connectivity` status signal is not
	// configurable and always collected.
	return []collection.Metric{
		{Name: "connectivity_latency_ms", Type: "gauge", Labels: map[string]string{}},
	}, nil
}
