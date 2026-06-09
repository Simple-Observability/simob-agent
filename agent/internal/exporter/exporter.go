package exporter

import (
	"encoding/json"
	"fmt"

	"agent/internal/config"
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

// TelemetryPayload represents a generic telemetry payload (e.g. process snapshots).
type TelemetryPayload struct {
	Timestamp string          `json:"timestamp"` // Unix timestamp in milliseconds as a string
	Type      string          `json:"type"`      // Type of telemetry (e.g. "processes")
	Data      json.RawMessage `json:"data"`      // Generic JSON payload
}

func (m MetricPayload) GetTimestamp() string    { return m.Timestamp }
func (l LogPayload) GetTimestamp() string       { return l.Timestamp }
func (t TelemetryPayload) GetTimestamp() string { return t.Timestamp }

// Exporter handles sending metrics and logs to remote storage.
type Exporter struct {
	spool   *spool
	flusher *flusher
}

// NewExporter creates a new Exporter instance.
// It loads configuration and initializes the HTTP client.
func NewExporter(cfg *config.Config, dryRun bool) (*Exporter, error) {
	return newExporter(cfg, dryRun, true)
}

// NewExporterWithoutFlusher creates a new Exporter instance that only spools payloads.
// Exported payloads are persisted locally until another process flushes the spool.
func NewExporterWithoutFlusher() (*Exporter, error) {
	return newExporter(nil, false, false)
}

func newExporter(cfg *config.Config, dryRun bool, startFlusher bool, opts ...spoolOption) (*Exporter, error) {
	spool, err := newSpool(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create spool instance: %w", err)
	}

	e := &Exporter{spool: spool}
	if !startFlusher {
		return e, nil
	}

	flusher, err := newFlusher(spool, cfg, dryRun)
	if err != nil {
		return nil, fmt.Errorf("failed to create flusher instance: %w", err)
	}

	e.flusher = flusher
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
		return fmt.Errorf("failed to append %d out of %d payloads", failed, len(metrics))
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
	logger.Log.Debug("Appended logs to spool", "count", len(logs), "failed", failed)
	if failed > 0 {
		return fmt.Errorf("failed to append %d out of %d payloads", failed, len(logs))
	}
	return nil
}

// ExportTelemetry sends a batch of telemetry payloads to the configured telemetry endpoint.
func (e *Exporter) ExportTelemetry(telemetry []TelemetryPayload) error {
	var failed int
	for _, t := range telemetry {
		if err := e.spool.append(t); err != nil {
			failed++
			logger.Log.Error("failed to append telemetry to spool", "error", err)
		}
	}
	logger.Log.Debug("Appended telemetry to spool", "count", len(telemetry), "failed", failed)
	if failed > 0 {
		return fmt.Errorf("failed to append %d out of %d payloads", failed, len(telemetry))
	}
	return nil
}

// Close gracefully shuts down the exporter
func (e *Exporter) Close() {
	if e.flusher != nil {
		e.flusher.stop()
	}
	e.spool.close()
}
