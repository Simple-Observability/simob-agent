package registry

import (
	"strings"

	"agent/internal/collection"
	"agent/internal/logger"
	"agent/internal/metrics"
	"agent/internal/metrics/apache"
	"agent/internal/metrics/caddy"
	"agent/internal/metrics/connectivity"
	"agent/internal/metrics/cpu"
	"agent/internal/metrics/disk"
	"agent/internal/metrics/memcached"
	"agent/internal/metrics/memory"
	"agent/internal/metrics/network"
	"agent/internal/metrics/nginx"
	"agent/internal/metrics/phpfpm"
	"agent/internal/metrics/status"
)

// metricsWithPrefix returns the config metrics whose name starts with prefix.
func metricsWithPrefix(cfg *collection.CollectionConfig, prefix string) []collection.Metric {
	var filtered []collection.Metric
	for _, m := range cfg.Metrics {
		if strings.HasPrefix(m.Name, prefix) {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

func BuildCollectors(cfg *collection.CollectionConfig) []metrics.MetricCollector {
	optInCollectors := map[string]metrics.MetricCollector{
		"apache":    apache.NewApacheCollector(),
		"caddy":     caddy.NewCaddyCollector(),
		"cpu":       cpu.NewCPUCollector(),
		"disk":      disk.NewDiskCollector(),
		"mem":       memory.NewMemoryCollector(),
		"memcached": memcached.NewMemcachedCollector(),
		"net":       network.NewNetworkCollector(),
		"nginx":     nginx.NewNginxCollector(),
		"phpfpm":    phpfpm.NewPHPFPMCollector(),
	}

	// Always-on collectors run regardless of the collection config.
	// Config metrics matching their name prefix are still forwarded to
	// them so their opt-in metrics (e.g. connectivity_latency_ms) can be
	// selected; each collector decides in Collect() what it emits
	// unconditionally (the `connectivity` status signal, heartbeat).
	alwaysOn := []metrics.MetricCollector{
		status.NewStatusCollector(),
		connectivity.NewConnectivityCollector(),
	}

	var allCollectors []metrics.MetricCollector
	allCollectors = append(allCollectors, alwaysOn...)

	// No config provided, return all collectors
	if cfg == nil {
		for prefix, collector := range optInCollectors {
			logger.Log.Debug("Including collector (no config)", "collector", prefix)
			allCollectors = append(allCollectors, collector)
		}
		return allCollectors
	}

	// Forward any selected metrics to the always-on collectors.
	// Collectors that ignore included metrics (status) are unaffected.
	for _, collector := range alwaysOn {
		filtered := metricsWithPrefix(cfg, collector.Name())
		if len(filtered) == 0 {
			continue
		}
		logger.Log.Debug("Assigned metrics to collector", "collector", collector.Name(), "count", len(filtered))
		collector.SetIncludedMetrics(filtered)
	}

	// Instantiate opt-in collectors only when the config selects their metrics.
	for prefix, collector := range optInCollectors {
		filtered := metricsWithPrefix(cfg, prefix)
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
