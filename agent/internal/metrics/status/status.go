package status

import (
	"time"

	"agent/internal/collection"
	"agent/internal/metrics"
)

type StatusCollector struct {
	metrics.BaseCollector
}

func NewStatusCollector() *StatusCollector {
	return &StatusCollector{}
}

func (c *StatusCollector) Name() string {
	return "status"
}

func (c *StatusCollector) Collect() ([]metrics.DataPoint, error) {
	return c.CollectAll()
}

func (c *StatusCollector) CollectAll() ([]metrics.DataPoint, error) {
	timestamp := time.Now().UnixMilli()

	return []metrics.DataPoint{
		{
			Name:      "heartbeat",
			Timestamp: timestamp,
			Value:     1,
			Labels:    map[string]string{},
		},
	}, nil
}

func (c *StatusCollector) Discover() ([]collection.Metric, error) {
	return []collection.Metric{}, nil
}
