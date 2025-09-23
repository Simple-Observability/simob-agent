package exporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"agent/internal/common"
	"agent/internal/config"
	"agent/internal/logger"
)

// Configuration constants
const (
	// FIXME: If inside flush interval there are more entries that MaxBatchSize,
	// it will accumulate entries. Flush routine should not exit until all elements are processed
	FlushInterval = 5 * time.Second
	MaxBatchSize  = 100
	MaxAge        = 24 * time.Hour
)
const (
	MetricsSpoolFileName = "metrics.jsonl"
	LogsSpoolFileName    = "logs.jsonl"
)

// Exporter handles sending metrics and logs to remote storage.
type Exporter struct {
	apiKey     string
	metricsURL string
	logsURL    string
	httpClient *http.Client
	dryRun     bool
	spoolDir   string // Buffer where payload are written before being flushed/exported
	ctx        context.Context
	cancel     context.CancelFunc
	flushers   []chan struct{} // Channels used to signal when a flusher stops
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
	Message   string            `json:"message"`
}

// Payload interface for generic handling
type Payload interface {
	GetTimestamp() string
}

func (m MetricPayload) GetTimestamp() string { return m.Timestamp }
func (l LogPayload) GetTimestamp() string    { return l.Timestamp }

// NewExporter creates a new Exporter instance.
// It loads configuration and initializes the HTTP client.
func NewExporter(dryRun bool) (*Exporter, error) {
	// Create spool directory
	programDirectory, err := common.GetProgramDirectory()
	if err != nil {
		return nil, fmt.Errorf("can't create spool directory. failed to get program directory: %w", err)
	}
	spoolDir := filepath.Join(programDirectory, "spool")
	err = os.MkdirAll(spoolDir, 0o770)
	if err != nil {
		return nil, fmt.Errorf("failed to create spool directory: %w", err)
	}

	// Create the dry run exporter
	if dryRun {
		return &Exporter{
			dryRun:   true,
			spoolDir: spoolDir,
		}, nil
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load exporter configuration: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	e := &Exporter{
		apiKey:     cfg.APIKey,
		metricsURL: cfg.MetricsExportUrl,
		logsURL:    cfg.LogsExportUrl,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		spoolDir: spoolDir,
		ctx:      ctx,
		cancel:   cancel,
	}
	// Start flusher goroutines
	e.startFlushers()
	return e, nil
}

// ExportMetric sends a batch of metrics to the configured metrics endpoint.
// The metrics should already be in the MetricPayload format.
func (e *Exporter) ExportMetric(metrics []MetricPayload) error {
	if len(metrics) == 0 {
		return nil // Nothing to export
	}

	spoolFile := filepath.Join(e.spoolDir, MetricsSpoolFileName)
	for _, metric := range metrics {
		err := e.appendToSpool(spoolFile, metric)
		if err != nil {
			logger.Log.Error("failed to append metric to spool", "error", err)
			return err
		}
	}
	logger.Log.Debug("Appended metrics to spool", "count", len(metrics))
	return nil
}

// ExportLog sends a batch of logs to the configured logs endpoint.
// The logs should already be in the LogPayload format.
func (e *Exporter) ExportLog(logs []LogPayload) error {
	if len(logs) == 0 {
		return nil // Nothing to export
	}

	spoolFile := filepath.Join(e.spoolDir, LogsSpoolFileName)
	for _, log := range logs {
		if err := e.appendToSpool(spoolFile, log); err != nil {
			logger.Log.Error("failed to append log to spool", "error", err)
			return err
		}
	}
	logger.Log.Debug("Appended logs to spool", "count", len(logs))
	return nil
}

// Close gracefully shuts down the exporter
func (e *Exporter) Close() error {
	if e.cancel != nil {
		logger.Log.Debug("Exporter received shutdown signal")
		e.cancel() // signals all flushers to stop
		for _, done := range e.flushers {
			<-done
		}
		logger.Log.Debug("Exporter shutdown complete")
	}
	return nil
}

// unmarshalMetric unmarshals a metric payload from JSON
func (e *Exporter) unmarshalMetric(data []byte) (Payload, error) {
	var metric MetricPayload
	if err := json.Unmarshal(data, &metric); err != nil {
		return nil, err
	}
	return metric, nil
}

// unmarshalLog unmarshals a log payload from JSON
func (e *Exporter) unmarshalLog(data []byte) (Payload, error) {
	var log LogPayload
	if err := json.Unmarshal(data, &log); err != nil {
		return nil, err
	}
	return log, nil
}

// ------------------------ Spool/Flushers ------------------------

// appendToSpool appends a single payload to the specified spool file
func (e *Exporter) appendToSpool(spoolFile string, payload Payload) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}
	f, err := os.OpenFile(spoolFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o660)
	if err != nil {
		return fmt.Errorf("failed to open spool file: %w", err)
	}
	defer f.Close()
	_, err = f.Write(append(payloadBytes, '\n'))
	if err != nil {
		return fmt.Errorf("failed to write to spool file: %w", err)
	}
	return nil
}

// rewriteSpool atomically rewrites the spool file with the given lines
func (e *Exporter) rewriteSpool(path string, lines []string) error {
	tmp := path + ".tmp"
	var content string
	if len(lines) > 0 {
		content = strings.Join(lines, "\n") + "\n"
	}
	if err := os.WriteFile(tmp, []byte(content), 0o660); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}
	return nil
}

// startFlushers launches the background flusher goroutines
func (e *Exporter) startFlushers() {
	// Start metrics flusher
	metricsDone := make(chan struct{})
	e.flushers = append(e.flushers, metricsDone)
	go e.startFlusher(
		filepath.Join(e.spoolDir, MetricsSpoolFileName),
		e.metricsURL,
		e.unmarshalMetric,
		metricsDone,
	)

	// Start logs flusher
	logsDone := make(chan struct{})
	e.flushers = append(e.flushers, logsDone)
	go e.startFlusher(
		filepath.Join(e.spoolDir, LogsSpoolFileName),
		e.logsURL,
		e.unmarshalLog,
		logsDone,
	)
}

// startFlusher lruns the periodic flush loop for a specific data type
func (e *Exporter) startFlusher(
	spoolFile string,
	url string,
	unmarshal func([]byte) (Payload, error),
	done chan struct{},
) {
	defer close(done)

	ticker := time.NewTicker(FlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-e.ctx.Done():
			// Final flush before shutdown
			e.flushAll(spoolFile, url, unmarshal)
			return
		case <-ticker.C:
			e.flushAll(spoolFile, url, unmarshal)
		}
	}
}

// flushAll processes all entries in the spool file, sending them in batches
// until the file is empty or context is cancelled
func (e *Exporter) flushAll(spoolFile string, url string, unmarshal func([]byte) (Payload, error)) {
	for {
		// Check if we should stop (context cancelled)
		select {
		case <-e.ctx.Done():
			return
		default:
		}

		// Try to process one batch
		hasMoreEntries, err := e.flushOnce(spoolFile, url, unmarshal)
		if err != nil {
			logger.Log.Error("error during flush", "file", spoolFile, "error", err)
			return
		}

		if !hasMoreEntries {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}
}

// flushOnce processed and sends a batch from the spool file
func (e *Exporter) flushOnce(spoolFile string, url string, unmarshal func([]byte) (Payload, error)) (bool, error) {
	// Read all lines from spool file
	data, err := os.ReadFile(spoolFile)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to read spool file: %w", err)
	}

	content := strings.TrimRight(string(data), "\n")
	if content == "" {
		return false, nil // Empty file
	}

	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return false, nil // No more entries
	}

	// Collect up to MaxBatchSize entries
	batchSize := min(len(lines), MaxBatchSize)
	rawBatch := lines[:batchSize]

	// Parse and filter stale entries
	var toSend []Payload
	var keepLines []string
	cutoff := time.Now().Add(-MaxAge).UnixMilli()
	for _, raw := range rawBatch {
		// Skip empty lines
		if strings.TrimSpace(raw) == "" {
			continue
		}
		// Skip corrupted entries
		obj, err := unmarshal([]byte(raw))
		if err != nil {
			logger.Log.Error("failed to unmarshal spool entry", "line", raw, "error", err)
			continue
		}
		// Skip stale (old) entries
		if t, err := strconv.ParseInt(obj.GetTimestamp(), 10, 64); err == nil && t < cutoff {
			logger.Log.Warn("skipping stale entry", "timestamp", obj.GetTimestamp())
			continue
		}
		toSend = append(toSend, obj)
	}

	// Keep all remaining lines beyond the batch
	if len(lines) > batchSize {
		keepLines = lines[batchSize:]
	}

	// Send batch if we have valid entries
	if len(toSend) > 0 {
		if err := e.sendPayload(url, toSend); err != nil {
			return false, fmt.Errorf("failed to send batch: %w", err)
		}
		logger.Log.Debug("successfully sent batch", "url", url, "count", len(toSend))
	}

	// Rewrite spool file with remaining entries
	if err := e.rewriteSpool(spoolFile, keepLines); err != nil {
		return false, fmt.Errorf("failed to rewrite spool file: %w", err)
	}

	// Return true if there are more entries to process
	return len(keepLines) > 0, nil
}

// ------------------------ Export ------------------------

// sendPayload is a private helper function to send JSON data to a given URL.
func (e *Exporter) sendPayload(url string, payload []Payload) error {
	// Dry run. Print payload without actually sending the request
	if e.dryRun {
		prettyPayload, err := json.MarshalIndent(payload, "", " ")
		if err != nil {
			logger.Log.Error("failed to pretty-print payload for dry-run", "error", err)
			return nil
		}
		fmt.Printf("[dry-run] Would send payload: %v\n", string(prettyPayload))
		return nil
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", e.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send data to %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("data export to %s failed with status code: %d", url, resp.StatusCode)
	}
	return nil
}
