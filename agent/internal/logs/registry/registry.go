package registry

import (
	"agent/internal/collection"
	"agent/internal/logger"
	"agent/internal/logs"
	"agent/internal/logs/apache"
	"agent/internal/logs/journalctl"
	"agent/internal/logs/nginx"
)

func BuildCollectors(cfg *collection.CollectionConfig) []logs.LogCollector {
	collectorMap := map[string]logs.LogCollector{
		"journalctl": journalctl.NewJournalCTLCollector(),
		"apache":     apache.NewApacheLogCollector(),
		"nginx":      nginx.NewNginxLogCollector(),
	}

	// If cfg is nil, return all collectors
	if cfg == nil {
		all := make([]logs.LogCollector, 0, len(collectorMap))
		for name, collector := range collectorMap {
			logger.Log.Debug("Including log collector (no config)", "name", name)
			all = append(all, collector)
		}
		return all
	}

	// Else, return only enabled ones
	enabled := make(map[string]bool)
	for _, src := range cfg.LogSources {
		enabled[src.Name] = true
	}
	var selected []logs.LogCollector
	for name, collector := range collectorMap {
		if enabled[name] {
			selected = append(selected, collector)
		} else {
			logger.Log.Debug("Skipping log collector", "name", name)
		}
	}

	return selected
}
