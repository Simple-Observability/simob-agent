package disk

import (
	"agent/internal/collection"
	"agent/internal/logger"
	"agent/internal/metrics"
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
)

type DiskCollector struct {
	metrics.BaseCollector
}

func NewDiskCollector() *DiskCollector {
	return &DiskCollector{}
}

func (c *DiskCollector) Name() string {
	return "disk"
}

// diskMetrics list the available metrics inside the disk package
var diskMetrics = []struct {
	name     string
	unit     string
	getValue func(*disk.UsageStat) float64
}{
	{"disk_total_bytes", "bytes", func(d *disk.UsageStat) float64 { return float64(d.Total) }},
	{"disk_free_bytes", "bytes", func(d *disk.UsageStat) float64 { return float64(d.Free) }},
	{"disk_used_bytes", "bytes", func(d *disk.UsageStat) float64 { return float64(d.Used) }},
	{"disk_used_ratio", "%", func(d *disk.UsageStat) float64 { return d.UsedPercent / 100 }},
	{"disk_inodes_total_total", "no", func(d *disk.UsageStat) float64 { return float64(d.InodesTotal) }},
	{"disk_inodes_free_total", "no", func(d *disk.UsageStat) float64 { return float64(d.InodesFree) }},
	{"disk_inodes_used_total", "no", func(d *disk.UsageStat) float64 { return float64(d.InodesUsed) }},
	{"disk_inodes_used_ratio", "%", func(d *disk.UsageStat) float64 { return d.InodesUsedPercent / 100 }},
}

func (c *DiskCollector) Collect() ([]metrics.DataPoint, error) {
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

func (c *DiskCollector) CollectAll() ([]metrics.DataPoint, error) {
	timestamp := time.Now().UnixMilli()

	partitions, err := disk.Partitions(false)
	if err != nil {
		return nil, fmt.Errorf("failed to get disk partitions info: %w", err)
	}

	var datapoints []metrics.DataPoint
	for _, p := range partitions {
		// Get usage stats
		usage, err := disk.Usage(p.Mountpoint)
		if err != nil {
			logger.Log.Error("failed to get usage stats", "mountpoint", p.Mountpoint)
			continue
		}
		labels := map[string]string{
			"device":     p.Device,
			"mountpoint": p.Mountpoint,
		}
		for _, m := range diskMetrics {
			datapoints = append(datapoints, metrics.DataPoint{
				Name:      m.name,
				Value:     m.getValue(usage),
				Timestamp: timestamp,
				Labels:    labels,
			})
		}
	}

	return datapoints, nil
}

func (c *DiskCollector) Discover() ([]collection.Metric, error) {
	partitions, err := disk.Partitions(false)
	if err != nil {
		return nil, fmt.Errorf("failed to discover disk partitions: %w", err)
	}

	var discovered []collection.Metric
	for _, p := range partitions {
		diskLabels := map[string]string{"device": p.Device, "mountpoint": p.Mountpoint}
		for _, m := range diskMetrics {
			discovered = append(discovered, collection.Metric{
				Name:   m.name,
				Type:   "gauge",
				Unit:   m.unit,
				Labels: diskLabels,
			})
		}
	}
	return discovered, nil
}
