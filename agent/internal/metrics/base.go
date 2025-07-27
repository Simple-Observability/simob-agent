package metrics

import "agent/internal/collection"

type BaseCollector struct {
	includedMetrics []collection.Metric
}

func (b *BaseCollector) SetIncludedMetrics(metrics []collection.Metric) {
	b.includedMetrics = metrics
}

// TODO: Use some sort of cache to avoid iterating over all the included metrics
func (b *BaseCollector) IsIncluded(name string, labels map[string]string) bool {
	for _, included := range b.includedMetrics {
		if name != included.Name {
			continue
		}
		if labelsEqual(labels, included.Labels) {
			return true
		}
	}
	return false
}

func labelsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
