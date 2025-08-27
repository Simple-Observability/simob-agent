package registry

import (
	"strings"

	"agent/internal/collection"
	"agent/internal/logger"
	"agent/internal/metrics"
	"agent/internal/metrics/cpu"
	"agent/internal/metrics/disk"
	"agent/internal/metrics/diskio"
	"agent/internal/metrics/memory"
	"agent/internal/metrics/network"
	"agent/internal/metrics/status"
)

func BuildCollectors(cfg *collection.CollectionConfig) []metrics.MetricCollector {
	collectorMap := map[string]metrics.MetricCollector{
		"cpu":    cpu.NewCPUCollector(),
		"mem":    memory.NewMemoryCollector(),
		"disk":   disk.NewDiskCollector(),
		"diskio": diskio.NewDiskIOCollector(),
		"net":    network.NewNetworkCollector(),
	}

	var allCollectors []metrics.MetricCollector
	allCollectors = append(allCollectors, status.NewStatusCollector())

	// No config provided, return all collectors
	if cfg == nil {
		for prefix, collector := range collectorMap {
			logger.Log.Debug("Including collector (no config)", "collector", prefix)
			allCollectors = append(allCollectors, collector)
		}
		return allCollectors
	}

	// Filter based on config
	for prefix, collector := range collectorMap {
		var filtered []collection.Metric
		for _, m := range cfg.Metrics {
			if strings.HasPrefix(m.Name, prefix) {
				filtered = append(filtered, m)
			}
		}

		if len(filtered) == 0 {
			logger.Log.Debug("Skipping collector with no included metrics", "collector", prefix)
			continue
		}

		logger.Log.Debug("Assigned metrics to collector", "collector", prefix, "count", len(filtered))
		collector.SetIncludedMetrics(filtered)
		allCollectors = append(allCollectors, collector)
	}
	return allCollectors
}
