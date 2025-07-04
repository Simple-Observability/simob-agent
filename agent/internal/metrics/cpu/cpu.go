package cpu

import (
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"

	"agent/internal/metrics"
)

type CPUCollector struct {
	lastStats []cpu.TimesStat
}

func NewCPUCollector() *CPUCollector {
	return &CPUCollector{}
}

func (c *CPUCollector) Name() string {
	return "cpu"
}

// FIXME: Important, SINGLE SOURCE OF TRUTH FOR MERTIC NAMES
func (c *CPUCollector) Collect() ([]metrics.DataPoint, error) {
	// Capture timestamp once for consistency across all datapoints
	timestamp := time.Now().UnixMilli()

	// Get current stats
	currStats, err := cpu.Times(true)
	if err != nil {
		return nil, fmt.Errorf("failed to get current CPU metrics: %w", err)
	}

	// First call: store initial stats and return no datapoints
	if c.lastStats == nil {
		c.lastStats = currStats
		return []metrics.DataPoint{}, nil
	}

	if len(currStats) != len(c.lastStats) {
		return nil, fmt.Errorf("CPU core count mismatch: previous=%d current=%d", len(c.lastStats), len(currStats))
	}

	var results []metrics.DataPoint

	// Initialize accumulators
	var totalUser float64
	var totalSystem float64
	var totalIdle float64
	var totalNice float64
	var totalIowait float64
	var totalIrq float64
	var totalSoftirq float64
	var totalSteal float64
	var totalGuest float64
	var totalGuestNice float64
	var totalAllCores float64

	// Process each core
	for i := range currStats {
		prev := c.lastStats[i]
		curr := currStats[i]

		// Compute deltas
		deltaUser := curr.User - prev.User
		deltaSystem := curr.System - prev.System
		deltaIdle := curr.Idle - prev.Idle
		deltaNice := curr.Nice - prev.Nice
		deltaIowait := curr.Iowait - prev.Iowait
		deltaIrq := curr.Irq - prev.Irq
		deltaSoftirq := curr.Softirq - prev.Softirq
		deltaSteal := curr.Steal - prev.Steal
		deltaGuest := curr.Guest - prev.Guest
		deltaGuestNice := curr.GuestNice - prev.GuestNice

		// Adjust user/nice
		adjustedUser := deltaUser - deltaGuest
		adjustedNice := deltaNice - deltaGuestNice

		// Compute total times
		totalCore := adjustedUser +
			deltaSystem +
			deltaIdle +
			adjustedNice +
			deltaIowait +
			deltaIrq +
			deltaSoftirq +
			deltaSteal +
			deltaGuest +
			deltaGuestNice

		// Avoid divide by zero
		if totalCore <= 0 {
			continue
		}

		// Accumulate totals
		totalUser += deltaUser
		totalSystem += deltaSystem
		totalIdle += deltaIdle
		totalNice += deltaNice
		totalIowait += deltaIowait
		totalIrq += deltaIrq
		totalSoftirq += deltaSoftirq
		totalSteal += deltaSteal
		totalGuest += deltaGuest
		totalGuestNice += deltaGuestNice
		totalAllCores += totalCore

		// Per-core metrics
		coreLabel := map[string]string{"cpu": curr.CPU}
		metricsToAdd := []struct {
			name  string
			value float64
		}{
			{"cpu_user_ratio", adjustedUser / totalCore},
			{"cpu_system_ratio", deltaSystem / totalCore},
			{"cpu_idle_ratio", deltaIdle / totalCore},
			{"cpu_nice_ratio", adjustedNice / totalCore},
			{"cpu_iowait_ratio", deltaIowait / totalCore},
			{"cpu_irq_ratio", deltaIrq / totalCore},
			{"cpu_softirq_ratio", deltaSoftirq / totalCore},
			{"cpu_steal_ratio", deltaSteal / totalCore},
			{"cpu_guest_ratio", deltaGuest / totalCore},
			{"cpu_guestNice_ratio", deltaGuestNice / totalCore},
		}
		for _, m := range metricsToAdd {
			results = append(results, metrics.DataPoint{
				Name:      m.name,
				Timestamp: timestamp,
				Value:     m.value,
				Labels:    coreLabel,
			})
		}
	}

	if totalAllCores > 0 {
		// Add total (all cores) metric
		allCoreLabels := map[string]string{"cpu": "all"}
		adjustedTotalUser := totalUser - totalGuest
		adjustedTotalNice := totalNice - totalGuestNice

		metricsToAdd := []struct {
			name  string
			value float64
		}{
			{"cpu_user_ratio", adjustedTotalUser / totalAllCores},
			{"cpu_system_ratio", totalSystem / totalAllCores},
			{"cpu_idle_ratio", totalIdle / totalAllCores},
			{"cpu_nice_ratio", adjustedTotalNice / totalAllCores},
			{"cpu_iowait_ratio", totalIowait / totalAllCores},
			{"cpu_irq_ratio", totalIrq / totalAllCores},
			{"cpu_softirq_ratio", totalSoftirq / totalAllCores},
			{"cpu_steal_ratio", totalSteal / totalAllCores},
			{"cpu_guest_ratio", totalGuest / totalAllCores},
			{"cpu_guestNice_ratio", totalGuestNice / totalAllCores},
		}
		for _, m := range metricsToAdd {
			results = append(results, metrics.DataPoint{
				Name:      m.name,
				Timestamp: timestamp,
				Value:     m.value,
				Labels:    allCoreLabels,
			})
		}
	}

	// Save stats
	c.lastStats = currStats

	return results, nil
}

func (c *CPUCollector) Discover() ([]metrics.Metric, error) {
	currStats, err := cpu.Times(true)
	if err != nil {
		return nil, fmt.Errorf("failed to discover CPU metrics: %w", err)
	}

	metricFields := []string{
		"user", "system", "idle", "nice", "iowait",
		"irq", "softirq", "steal", "guest", "guestNice",
	}
	var discovered []metrics.Metric
	// Discover per-core metrics
	for _, core := range currStats {
		for _, field := range metricFields {
			discovered = append(discovered, metrics.Metric{
				Name:   "cpu_" + field + "_ratio",
				Type:   "gauge",
				Unit:   "%",
				Labels: map[string]string{"cpu": core.CPU},
			})
		}
	}

	// Add aggregate metrics once
	for _, field := range metricFields {
		discovered = append(discovered, metrics.Metric{
			Name:   "cpu_" + field + "_ratio",
			Type:   "gauge",
			Unit:   "%",
			Labels: map[string]string{"cpu": "total"},
		})
	}

	return discovered, nil
}
