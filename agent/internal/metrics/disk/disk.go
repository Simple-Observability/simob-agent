package disk

import (
	"fmt"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/disk"

	"agent/internal/collection"
	"agent/internal/logger"
	"agent/internal/metrics"
)

type DiskPS interface {
	Partitions(all bool) ([]disk.PartitionStat, error)
	Usage(path string) (*disk.UsageStat, error)
	IOCounters(names ...string) (map[string]disk.IOCountersStat, error)
}

type systemPS struct{}

func (s *systemPS) Partitions(all bool) ([]disk.PartitionStat, error) {
	return disk.Partitions(all)
}

func (s *systemPS) Usage(path string) (*disk.UsageStat, error) {
	return disk.Usage(path)
}

func (s *systemPS) IOCounters(names ...string) (map[string]disk.IOCountersStat, error) {
	return disk.IOCounters(names...)
}

type DiskCollector struct {
	metrics.BaseCollector

	ps        DiskPS
	lastStats map[string]disk.IOCountersStat
	lastTime  int64
	now       func() int64
}

func NewDiskCollector() *DiskCollector {
	return &DiskCollector{
		ps:        &systemPS{},
		lastStats: make(map[string]disk.IOCountersStat),
		now:       func() int64 { return time.Now().UnixMilli() },
	}
}

func (c *DiskCollector) Name() string {
	return "disk"
}

// normalizeDeviceName strips the common '/dev/' prefix from a device path
// on Unix-like systems (Linux, macOS, etc.) to align partition device names
// with I/O counter device names. On Windows, the path is returned unchanged,
// as the /dev/ prefix is not used in its device paths, ensuring Windows
// device identifiers remain intact.
func normalizeDeviceName(devicePath string) string {
	if runtime.GOOS == "windows" {
		return devicePath
	}
	return strings.TrimPrefix(devicePath, "/dev/")
}

// getUniquePrimaryPartitions fetches all partitions, then filters them to ensure:
// 1. Bind mounts are skipped (via "bind" option).
// 2. Only the first encountered partition for a given underlying block device is included.
func (c *DiskCollector) getUniquePrimaryPartitions() ([]disk.PartitionStat, error) {
	partitions, err := c.ps.Partitions(false)
	if err != nil {
		return nil, err
	}
	var uniquePartitions []disk.PartitionStat
	processedDevices := make(map[string]struct{})

	for _, p := range partitions {
		// 1. Skip bind mounts
		if slices.Contains(p.Opts, "bind") {
			continue
		}

		// 2. Enforce uniqueness of the underlying block device
		deviceName := normalizeDeviceName(p.Device)
		if _, exists := processedDevices[deviceName]; exists {
			continue
		}
		processedDevices[deviceName] = struct{}{}

		uniquePartitions = append(uniquePartitions, p)
	}

	return uniquePartitions, nil
}

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

var diskIOMetrics = []struct {
	name     string
	unit     string
	getValue func(current, previous *disk.IOCountersStat, deltaT float64) float64
}{
	{
		"disk_read_rate", "rate",
		func(current, previous *disk.IOCountersStat, deltaT float64) float64 {
			delta := float64(current.ReadCount - previous.ReadCount)
			return delta / deltaT * 1000.0
		},
	},
	{
		"disk_write_rate", "rate",
		func(current, previous *disk.IOCountersStat, deltaT float64) float64 {
			delta := float64(current.WriteCount - previous.WriteCount)
			return delta / deltaT * 1000.0
		},
	},
	{
		"disk_read_bps", "bps",
		func(current, previous *disk.IOCountersStat, deltaT float64) float64 {
			delta := float64(current.ReadBytes - previous.ReadBytes)
			return delta / deltaT * 1000.0
		},
	},
	{
		"disk_write_bps", "bps",
		func(current, previous *disk.IOCountersStat, deltaT float64) float64 {
			delta := float64(current.WriteBytes - previous.WriteBytes)
			return delta / deltaT * 1000.0
		},
	},
	{
		"disk_busy_ratio", "%",
		func(current, previous *disk.IOCountersStat, deltaT float64) float64 {
			deltaIoTime := float64(current.IoTime - previous.IoTime)
			ratio := deltaIoTime / deltaT
			return min(1.0, ratio)
		},
	},
	{
		"disk_avg_request_ms", "ms",
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
	now := c.now
	if now == nil {
		now = func() int64 { return time.Now().UnixMilli() }
	}
	timestamp := now()

	partitions, err := c.getUniquePrimaryPartitions()
	if err != nil {
		return nil, fmt.Errorf("failed to get unique primary partitions: %w", err)
	}

	currentIOCounters, err := c.ps.IOCounters()
	if err != nil {
		return nil, fmt.Errorf("failed to get disk I/O info: %w", err)
	}

	deltaT := timestamp - c.lastTime
	var datapoints []metrics.DataPoint
	for _, p := range partitions {
		// Collect usage metrics
		usage, err := c.ps.Usage(p.Mountpoint)
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

		// Collect IO metrics
		deviceName := normalizeDeviceName(p.Device)
		currentIO, ioExists := currentIOCounters[deviceName]
		previousIO, ioWasTracked := c.lastStats[deviceName]

		if ioExists && ioWasTracked && deltaT > 0 {
			for _, m := range diskIOMetrics {
				datapoints = append(datapoints, metrics.DataPoint{
					Name:      m.name,
					Value:     m.getValue(&currentIO, &previousIO, float64(deltaT)),
					Timestamp: timestamp,
					Labels:    labels,
				})
			}
		}
	}

	c.lastStats = currentIOCounters
	c.lastTime = timestamp

	return datapoints, nil
}

func (c *DiskCollector) Discover() ([]collection.Metric, error) {
	partitions, err := c.getUniquePrimaryPartitions()
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
		for _, m := range diskIOMetrics {
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
