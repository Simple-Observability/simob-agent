package disk

import (
	"agent/internal/logger"
	"agent/internal/metrics"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
)

type DiskCollector struct {
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
	timestamp := time.Now().UnixMilli()

	partitions, err := disk.Partitions(false)
	if err != nil {
		return nil, fmt.Errorf("failed to get disk partitions info: %w", err)
	}

	var datapoints []metrics.DataPoint
	for _, p := range partitions {
		if isValidPartition(p) {
			// Get usage stats for the valid mountpoint
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
	}

	return datapoints, nil
}

func (c *DiskCollector) Discover() ([]metrics.Metric, error) {
	partitions, err := disk.Partitions(false)
	if err != nil {
		return nil, fmt.Errorf("failed to discover disk partitions: %w", err)
	}

	var discovered []metrics.Metric
	for _, p := range partitions {
		if isValidPartition(p) {
			diskLabels := map[string]string{"device": p.Device, "mountpoint": p.Mountpoint}
			for _, m := range diskMetrics {
				discovered = append(discovered, metrics.Metric{
					Name:   m.name,
					Type:   "gauge",
					Unit:   m.unit,
					Labels: diskLabels,
				})
			}
		}
	}
	return discovered, nil
}

func isValidPartition(p disk.PartitionStat) bool {
	excludedFstypes := map[string]bool{
		"tmpfs":    true, // Temporary filesystem in memory
		"devtmpfs": true, // Device nodes in /dev
		"proc":     true, // Process info pseudo-FS
		"sysfs":    true, // Kernel objects pseudo-FS
		"devfs":    true, // macOS device FS
		"autofs":   true, // Automounter FS
		"overlay":  true, // OverlayFS used by containers
		"cgroup":   true, // Control group FS
	}
	// Skip pseudo and virtual filesystems that are not relevant for usage monitoring
	if excludedFstypes[p.Fstype] {
		return false
	}
	// Ignore partitions mounted under /proc or /sys (Linux virtual FS paths)
	if strings.HasPrefix(p.Mountpoint, "/proc") || strings.HasPrefix(p.Mountpoint, "/sys") {
		return false
	}
	// On macOS, ignore system-specific partitions except /System/Volumes/Data
	if runtime.GOOS == "darwin" && strings.HasPrefix(p.Mountpoint, "/System/Volumes") && p.Mountpoint != "/System/Volumes/Data" {
		return false
	}
	// Ignore read-only partitions
	if slices.Contains(p.Opts, "ro") {
		return false
	}

	return true
}
