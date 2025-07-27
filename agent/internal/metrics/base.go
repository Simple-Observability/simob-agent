package metrics

import "agent/internal/collection"

type BaseCollector struct {
	includedMetrics []collection.Metric
}

func (b *BaseCollector) SetIncludedMetrics(metrics []collection.Metric) {
	b.includedMetrics = metrics
}
