package diskio

import (
	"agent/internal/collection"
	"agent/internal/metrics"
	"encoding/json"
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
)

type DiskIOCollector struct {
	metrics.BaseCollector

	lastStats map[string]disk.IOCountersStat
	lastTime  int64
}

func NewDiskIOCollector() *DiskIOCollector {
	return &DiskIOCollector{}
}

func (c *DiskIOCollector) Name() string {
	return "diskio"
}

// diskIOMetrics list the available metrics inside the diskio package
var diskIOMetrics = []struct {
	name     string
	unit     string
	getValue func(current, previous *disk.IOCountersStat, deltaT float64) float64
}{
	{
		"diskio_read_rate", "rate",
		func(current, previous *disk.IOCountersStat, deltaT float64) float64 {
			delta := float64(current.ReadCount - previous.ReadCount)
			return delta / deltaT * 1000.0
		},
	},
	{
		"diskio_write_rate", "rate",
		func(current, previous *disk.IOCountersStat, deltaT float64) float64 {
			delta := float64(current.WriteCount - previous.WriteCount)
			return delta / deltaT * 1000.0
		},
	},
	{
		"diskio_read_bps", "bps",
		func(current, previous *disk.IOCountersStat, deltaT float64) float64 {
			delta := float64(current.ReadBytes - previous.ReadBytes)
			return delta / deltaT * 1000.0
		},
	},
	{
		"diskio_write_bps", "bps",
		func(current, previous *disk.IOCountersStat, deltaT float64) float64 {
			delta := float64(current.WriteBytes - previous.WriteBytes)
			return delta / deltaT * 1000.0
		},
	},
	{
		"diskio_used_ratio", "%",
		func(current, previous *disk.IOCountersStat, deltaT float64) float64 {
			deltaIoTime := float64(current.IoTime - previous.IoTime)
			ratio := deltaIoTime / deltaT
			return min(1.0, ratio)
		},
	},
	{
		"diskio_avg_request_ms", "ms",
		func(current, previous *disk.IOCountersStat, deltaT float64) float64 {
			deltaReadTime := float64(current.ReadTime - previous.ReadTime)
			deltaWriteTime := float64(current.WriteTime - previous.WriteTime)
			deltaReadCount := float64(current.ReadCount - previous.ReadCount)
			deltaWriteCount := float64(current.WriteCount - previous.WriteCount)

			totalTime := deltaReadTime + deltaWriteTime
			totalOps := deltaReadCount + deltaWriteCount

			if totalOps == 0 {
				return 0
			}
			return totalTime / totalOps
		},
	},
}

func (c *DiskIOCollector) Collect() ([]metrics.DataPoint, error) {
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

func (c *DiskIOCollector) CollectAll() ([]metrics.DataPoint, error) {
	timestamp := time.Now().UnixMilli()
	ioStats, err := disk.IOCounters()
	if err != nil {
		return nil, fmt.Errorf("failed to collect disk IO stats: %w", err)
	}

	if c.lastStats == nil {
		c.lastStats = make(map[string]disk.IOCountersStat)
		for name, s := range ioStats {
			c.lastStats[name] = s
		}
		c.lastTime = timestamp
		return []metrics.DataPoint{}, nil
	}

	deltaT := timestamp - c.lastTime
	if deltaT <= 0 {
		return nil, nil
	}

	var results []metrics.DataPoint

	for name, s := range ioStats {
		prev, ok := c.lastStats[name]
		if !ok {
			continue
		}

		labels := map[string]string{"device": name}

		for _, m := range diskIOMetrics {
			value := m.getValue(&s, &prev, float64(deltaT))
			results = append(results, metrics.DataPoint{
				Name:      m.name,
				Timestamp: timestamp,
				Value:     value,
				Labels:    labels,
			})
		}

		c.lastStats[name] = s
	}

	c.lastTime = timestamp

	b, _ := json.MarshalIndent(results, "", "  ")
	fmt.Println(string(b))
	return results, nil
}

func (c *DiskIOCollector) Discover() ([]collection.Metric, error) {
	ioStats, err := disk.IOCounters()
	if err != nil {
		return nil, fmt.Errorf("failed to discover disk IO devices: %w", err)
	}

	var discovered []collection.Metric
	for diskName := range ioStats {
		labels := map[string]string{"device": diskName}
		for _, m := range diskIOMetrics {
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
