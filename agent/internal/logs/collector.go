package logs

import (
	"context"
	"strconv"
	"sync"

	"agent/internal/collection"
	"agent/internal/exporter"
	"agent/internal/logger"
)

// RawLogLine carries a raw log line and its origin
// and is emitted by log collectors
type RawLogLine struct {
	Text   string
	Source string
}

// LogEntry represents a single log entry with extracted labels
type LogEntry struct {
	Timestamp int64             // Unix timestamp in milliseconds
	Source    string            // Source file path
	Text      string            // Raw log message
	Labels    map[string]string // Key-value pairs for labels
}

// LogCollector defines the interface for logs collection implementations.
type LogCollector interface {
	// Name returns the collector's identifier (e.g., "nginx", "apache").
	Name() string

	// Discover reports the available log sources this collector can produce
	// It is called during agent initialization to inform config/build process.
	Discover() []collection.LogSource

	// Start begins the log collection process for all discovered log sources.
	// This could involve tailing files, polling APIs, or listening to sockets.
	Start(ctx context.Context, out chan<- LogEntry) error

	// Stop terminates the log collection process and performs any necessary cleanup
	Stop() error
}

// StartCollection is the orchestrator that launches all collectors,
// parses raw lines into entries, and exports them.
func StartCollection(
	collectors []LogCollector,
	ctx context.Context,
	wg *sync.WaitGroup,
	exp *exporter.Exporter,
) {
	defer wg.Done()

	// Create shared channel
	logsChan := make(chan LogEntry, 1000)

	// Start all collectors
	for _, c := range collectors {
		err := c.Start(ctx, logsChan)
		if err != nil {
			logger.Log.Error("failed to start log collector", "name", c.Name(), "error", err)
		}
	}

	// Processing loop (parse + export)
	go func() {
		for {
			select {
			case <-ctx.Done():
				close(logsChan)
				return

			case logEntry, ok := <-logsChan:
				if !ok {
					// triggers when channel is closed
					return
				}
				logger.Log.Debug("Logs collected", "source", logEntry.Source)
				logPayload := convertLogEntryToPayload(logEntry)
				logPayloadList := []exporter.LogPayload{logPayload}
				err := exp.ExportLog(logPayloadList)
				if err != nil {
					logger.Log.Error("failed to export logs payload", "error", err)
				}
			}
		}
	}()

	// Stop for exit signal to stop all collectors
	<-ctx.Done()
	logger.Log.Info("Logs collection received stop signal.")
	exp.Close()
	for _, c := range collectors {
		c.Stop()
	}
}

func DiscoverAvailableLogSources(collectors []LogCollector) []collection.LogSource {
	var results []collection.LogSource
	for _, collector := range collectors {
		discovered := collector.Discover()
		results = append(results, discovered...)
	}
	return results
}

func convertLogEntryToPayload(entry LogEntry) exporter.LogPayload {
	// Clone labels to avoid mutating the original map
	labels := make(map[string]string, len(entry.Labels)+1)
	for k, v := range entry.Labels {
		labels[k] = v
	}

	// Add source to labels
	labels["source"] = entry.Source

	return exporter.LogPayload{
		Timestamp: strconv.FormatInt(entry.Timestamp, 10),
		Labels:    labels,
		Message:   entry.Text,
	}
}
