package phpfpm

import (
	"fmt"
	"time"

	"agent/internal/collection"
	"agent/internal/logger"
	"agent/internal/metrics"
)

type PHPFPMClient interface {
	GetStats() (*FPMStatus, error)
}

type FPMStatus struct {
	Timestamp          int64  `json:"-"`
	Pool               string `json:"pool"`
	ProcessManager     string `json:"process manager"`
	ListenQueue        uint64 `json:"listen queue"`
	MaxListenQueue     uint64 `json:"max listen queue"`
	ListenQueueLen     uint64 `json:"listen queue len"`
	IdleProcesses      uint64 `json:"idle processes"`
	ActiveProcesses    uint64 `json:"active processes"`
	TotalProcesses     uint64 `json:"total processes"`
	MaxActiveProcesses uint64 `json:"max active processes"`
	AcceptedConn       uint64 `json:"accepted conn"`
	MaxChildrenReached uint64 `json:"max children reached"`
	SlowRequests       uint64 `json:"slow requests"`
}

type Collector struct {
	metrics.BaseCollector

	client    PHPFPMClient
	lastStats *FPMStatus
	now       func() time.Time
}

func NewPHPFPMCollector() *Collector {
	return &Collector{
		client: newDefaultFastCGIClient(),
		now:    time.Now,
	}
}

func (c *Collector) Name() string {
	return "phpfpm"
}

var metricDefinitions = []struct {
	name   string
	kind   string
	getVal func(current, previous *FPMStatus) float64
}{
	{
		name:   "phpfpm_listen_queue",
		kind:   "gauge",
		getVal: func(current, previous *FPMStatus) float64 { return float64(current.ListenQueue) },
	},
	{
		name:   "phpfpm_max_listen_queue",
		kind:   "gauge",
		getVal: func(current, previous *FPMStatus) float64 { return float64(current.MaxListenQueue) },
	},
	{
		name:   "phpfpm_listen_queue_len",
		kind:   "gauge",
		getVal: func(current, previous *FPMStatus) float64 { return float64(current.ListenQueueLen) },
	},
	{
		name:   "phpfpm_idle_processes",
		kind:   "gauge",
		getVal: func(current, previous *FPMStatus) float64 { return float64(current.IdleProcesses) },
	},
	{
		name:   "phpfpm_active_processes",
		kind:   "gauge",
		getVal: func(current, previous *FPMStatus) float64 { return float64(current.ActiveProcesses) },
	},
	{
		name:   "phpfpm_total_processes",
		kind:   "gauge",
		getVal: func(current, previous *FPMStatus) float64 { return float64(current.TotalProcesses) },
	},
	{
		name:   "phpfpm_max_active_processes",
		kind:   "gauge",
		getVal: func(current, previous *FPMStatus) float64 { return float64(current.MaxActiveProcesses) },
	},
	{
		name: "phpfpm_accepted_conn_rate",
		kind: "gauge",
		getVal: func(current, previous *FPMStatus) float64 {
			if previous == nil {
				return 0
			}
			return counterRate(current.Timestamp, current.AcceptedConn, previous.Timestamp, previous.AcceptedConn)
		},
	},
	{
		name:   "phpfpm_max_children_reached_total",
		kind:   "counter",
		getVal: func(current, previous *FPMStatus) float64 { return float64(current.MaxChildrenReached) },
	},
	{
		name: "phpfpm_slow_requests_rate",
		kind: "gauge",
		getVal: func(current, previous *FPMStatus) float64 {
			if previous == nil {
				return 0
			}
			return counterRate(current.Timestamp, current.SlowRequests, previous.Timestamp, previous.SlowRequests)
		},
	},
}

func (c *Collector) Collect() ([]metrics.DataPoint, error) {
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

func (c *Collector) CollectAll() ([]metrics.DataPoint, error) {
	stats, err := c.getStats()
	if err != nil {
		logger.Log.Debug("Failed to collect metrics", "collector", c.Name(), "error", err)
		return nil, nil
	}

	var results []metrics.DataPoint
	for _, metricDef := range metricDefinitions {
		results = append(results, metrics.DataPoint{
			Name:      metricDef.name,
			Timestamp: stats.Timestamp,
			Value:     metricDef.getVal(stats, c.lastStats),
			Labels:    map[string]string{},
		})
	}

	c.lastStats = stats

	return results, nil
}

func (c *Collector) Discover() ([]collection.Metric, error) {
	if _, err := c.getStats(); err != nil {
		return nil, nil
	}

	var discovered []collection.Metric
	for _, metricDef := range metricDefinitions {
		discovered = append(discovered, collection.Metric{
			Name:   metricDef.name,
			Type:   metricDef.kind,
			Labels: map[string]string{},
		})
	}

	return discovered, nil
}

func (c *Collector) getStats() (*FPMStatus, error) {
	stats, err := c.client.GetStats()
	if err != nil {
		return nil, fmt.Errorf("failed to get php-fpm stats: %w", err)
	}
	stats.Timestamp = c.now().UnixMilli()
	return stats, nil
}

func counterRate(currentTs int64, current uint64, previousTs int64, previous uint64) float64 {
	deltaMs := currentTs - previousTs
	if deltaMs <= 0 {
		return 0
	}
	var delta float64
	if current < previous {
		delta = float64(current)
	} else {
		delta = float64(current - previous)
	}
	return delta / float64(deltaMs) * 1000
}
