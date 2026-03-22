package exporter

import (
	"fmt"

	"agent/internal/logger"
)

// Payload interface for generic handling
type Payload interface {
	GetTimestamp() string
}

// MetricPayload represents the structure required for metric data export.
type MetricPayload struct {
	Timestamp string            `json:"timestamp"` // Unix timestamp in milliseconds as a string
	Labels    map[string]string `json:"labels"`
	Name      string            `json:"name"`
	Value     float64           `json:"value"`
}

// LogPayload represents the structure required for log data export.
type LogPayload struct {
	Timestamp string            `json:"timestamp"` // Unix timestamp in milliseconds as a string
	Labels    map[string]string `json:"labels"`
	Metadata  map[string]string `json:"metadata"`
	Message   string            `json:"message"`
}

func (m MetricPayload) GetTimestamp() string { return m.Timestamp }
func (l LogPayload) GetTimestamp() string    { return l.Timestamp }

// Exporter handles sending metrics and logs to remote storage.
type Exporter struct {
	spool   *spool
	flusher *flusher
}

// NewExporter creates a new Exporter instance.
// It loads configuration and initializes the HTTP client.
func NewExporter(dryRun bool) (*Exporter, error) {
	spool, err := newSpool()
	if err != nil {
		return nil, fmt.Errorf("failed to create spool instance: %w", err)
	}

	flusher, err := newFlusher(spool, dryRun)
	if err != nil {
		return nil, fmt.Errorf("failed to create flusher instance: %w", err)
	}

	e := &Exporter{spool: spool, flusher: flusher}
	e.flusher.start()
	return e, nil
}

// ExportMetric sends a batch of metrics to the configured metrics endpoint.
// The metrics should already be in the MetricPayload format.
func (e *Exporter) ExportMetric(metrics []MetricPayload) error {
	var failed int
	for _, metric := range metrics {
		if err := e.spool.append(metric); err != nil {
			failed++
			logger.Log.Error("failed to append metric to spool", "error", err)
		}
	}
	logger.Log.Debug("Appended metrics to spool", "count", len(metrics), "failed", failed)
	if failed > 0 {
		return fmt.Errorf("failed to append %d out of %d metrics", failed, len(metrics))
	}
	return nil
}

// ExportLog sends a batch of logs to the configured logs endpoint.
// The logs should already be in the LogPayload format.
func (e *Exporter) ExportLog(logs []LogPayload) error {
	var failed int
	for _, log := range logs {
		if err := e.spool.append(log); err != nil {
			failed++
			logger.Log.Error("failed to append log to spool", "error", err)
		}
	}
	logger.Log.Debug("Appended metrics to spool", "count", len(logs), "failed", failed)
	if failed > 0 {
		return fmt.Errorf("failed to append %d out of %d metrics", failed, len(logs))
	}
	return nil
}

// Close gracefully shuts down the exporter
func (e *Exporter) Close() {
	e.flusher.stop()
	e.spool.close()
}
