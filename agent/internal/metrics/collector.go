package metrics

import (
	"context"
	"strconv"
	"sync"
	"time"

	"agent/internal/collection"
	"agent/internal/exporter"
	"agent/internal/logger"
)

// DataPoint represent a single measurement of a metric
type DataPoint struct {
	Name      string            `json:"name"`
	Timestamp int64             `json:"timestamp"` // Unix timestamp in milliseconds
	Value     float64           `json:"value"`
	Labels    map[string]string `json:"labels"`
}

// MetricCollector defines the interface for metric collection implementations.
type MetricCollector interface {
	// Name returns the collector's identifier (e.g., "cpu", "memory").
	Name() string

	// Discover reports the available metric this collector can produce
	// It is called during agent initialization to inform config/build process.
	Discover() ([]collection.Metric, error)

	// Collect gathers metrics and returns them as a slice of generic data points.
	Collect() ([]DataPoint, error)

	SetIncludedMetrics(metrics []collection.Metric)
}

// StartCollection initialize a background metrics collection loop that gatherns metrics from a list
// of provided collectors at the specified interval. The loop runs until the provided context is cancelled.
// After exiting, it signal completion to the wait group.
func StartCollection(
	collectors []MetricCollector,
	interval time.Duration,
	ctx context.Context,
	wg *sync.WaitGroup,
	exporter *exporter.Exporter,
) {
	// Signal completion on exit
	defer wg.Done()

	// Create ticker and ensure is stopped when function exits
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Infinite loop
	for {
		select {
		// Perform collection when the ticker fires
		case <-ticker.C:
			metrics := performCollection(collectors)
			payload := convertDataPointsToPayloads(metrics)
			err := exporter.ExportMetric(payload)
			if err != nil {
				logger.Log.Error("failed to export metrics payload", "error", err)
			}
			logger.Log.Debug("Metrics collected", "count", len(metrics))

		// Exit loop when stop signal fires
		case <-ctx.Done():
			logger.Log.Info("Metrics collection received stop signal.")
			return
		}
	}
}

// discoverAvailableMetrics runs discovery on all collectors and returns all available metrics.
func DiscoverAvailableMetrics(collectors []MetricCollector) []collection.Metric {
	var results []collection.Metric
	for _, collector := range collectors {
		discovered, err := collector.Discover()
		if err != nil {
			// Log error and try with next collector
			logger.Log.Error("failed to discover available metrics", "collector", collector.Name(), "error", err)
			continue
		}
		results = append(results, discovered...)
	}
	return results
}

// performCollection executes collection across all provided collectors and aggregates results.
func performCollection(collectors []MetricCollector) []DataPoint {
	var collectedMetrics []DataPoint
	for _, c := range collectors {
		datapoint, err := c.Collect()
		if err != nil {
			// Log error and try with next collector
			logger.Log.Error("failed to collect metrics", "collector", c.Name(), "error", err)
			continue
		}
		collectedMetrics = append(collectedMetrics, datapoint...)
	}
	return collectedMetrics
}

func convertDataPointsToPayloads(dps []DataPoint) []exporter.MetricPayload {
	out := make([]exporter.MetricPayload, 0, len(dps))
	for _, dp := range dps {
		out = append(out, exporter.MetricPayload{
			Timestamp: strconv.FormatInt(dp.Timestamp, 10),
			Labels:    dp.Labels,
			Name:      dp.Name,
			Value:     dp.Value,
		})
	}
	return out
}
