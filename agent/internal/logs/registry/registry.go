package registry

import (
	"agent/internal/collection"
	"agent/internal/logger"
	"agent/internal/logs"
	"agent/internal/logs/nginx"
)

func BuildCollectors(cfg *collection.CollectionConfig) []logs.LogCollector {
	collectorMap := map[string]logs.LogCollector{
		"nginx": nginx.NewNginxLogCollector(),
	}

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
