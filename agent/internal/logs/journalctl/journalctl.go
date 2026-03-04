package journalctl

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"agent/internal/collection"
	"agent/internal/logger"
	"agent/internal/logs"
)

var severityMap = map[int]string{
	0: "emergency",
	1: "alert",
	2: "critical",
	3: "error",
	4: "warning",
	5: "notice",
	6: "info",
	7: "debug",
}

const defaultSeverity = 6

type JournalCTLCollector struct {
	name    string
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running bool
	mu      sync.Mutex
}

func NewJournalCTLCollector() *JournalCTLCollector {
	return &JournalCTLCollector{
		name: "journalctl",
	}
}

func (c *JournalCTLCollector) Name() string {
	return c.name
}

func (c *JournalCTLCollector) Discover() []collection.LogSource {
	// Check if journalctl binary is available
	_, err := exec.LookPath("journalctl")
	if err != nil {
		return []collection.LogSource{}
	}

	// Check if we can actually run journalctl (permissions and reachability)
	err = exec.Command("journalctl", "-n", "0").Run()
	if err != nil {
		logger.Log.Debug("journalctl binary exists but cannot be executed properly", "error", err)
		return []collection.LogSource{}
	}

	return []collection.LogSource{
		{
			Name: c.name,
			Path: "",
		},
	}
}

func (c *JournalCTLCollector) Start(ctx context.Context, out chan<- logs.LogEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return fmt.Errorf("journalctl collector already running")
	}

	c.running = true

	// Create a child context so the collector can be stopped independently via
	// c.cancel while still respecting cancellation from the parent context.
	collectorCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	c.wg.Add(1)
	go c.readJournalLoop(collectorCtx, out)

	return nil
}

func (c *JournalCTLCollector) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cancel != nil {
		c.cancel()
	}

	// Wait for the goroutine to finish
	c.wg.Wait()
	c.running = false
	c.cancel = nil

	return nil
}

func (c *JournalCTLCollector) readJournalLoop(ctx context.Context, out chan<- logs.LogEntry) {
	defer c.wg.Done()
	for {
		err := c.runJournalctl(ctx, out)
		if err != nil {
			// Do not log context cancellation as an error since it's expected during shutdown
			if ctx.Err() == nil {
				logger.Log.Error("journalctl process exited with error", "error", err)
			}
		} else {
			logger.Log.Debug("journalctl process exited normally")
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
			// retry backoff before restarting journalctl
		}
	}
}

func (c *JournalCTLCollector) runJournalctl(ctx context.Context, out chan<- logs.LogEntry) error {
	cmd := exec.CommandContext(ctx, "journalctl", "-n", "0", "-f", "-o", "json")
	cmd.WaitDelay = 5 * time.Second

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start journalctl: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	// journalctl lines can be quite large, increase buffer capacity if needed
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		line := scanner.Bytes()

		logEntry, err := c.processJSONEntry(line)
		if err != nil {
			logger.Log.Error("failed to process journalctl entry", "error", err)
			continue
		}

		out <- logEntry
	}

	if err := scanner.Err(); err != nil {
		logger.Log.Error("scanner error reading journalctl stdout", "error", err)
	}

	return cmd.Wait()
}

func (c *JournalCTLCollector) processJSONEntry(line []byte) (logs.LogEntry, error) {
	logEntry := logs.LogEntry{
		Source: c.name,
		Labels: make(map[string]string),
	}

	var parsedEntry map[string]JournalField
	if err := json.Unmarshal(line, &parsedEntry); err != nil {
		return logEntry, fmt.Errorf("json unmarshal: %w", err)
	}

	// Parse timestamp
	tsField := parsedEntry["__REALTIME_TIMESTAMP"]
	tsStr := tsField.First()
	if tsStr != "" {
		if tsMicro, err := strconv.ParseInt(tsStr, 10, 64); err == nil {
			logEntry.Timestamp = tsMicro / 1000
		}
	}

	if logEntry.Timestamp == 0 {
		logEntry.Timestamp = time.Now().UnixMilli()
	}

	// Parse priority/severity
	priorityField := parsedEntry["PRIORITY"]
	priorityStr := priorityField.First()
	priorityInt := defaultSeverity
	if priorityStr != "" {
		if p, err := strconv.Atoi(priorityStr); err == nil {
			priorityInt = p
		} else {
			logger.Log.Debug("can't process priority. using fallback value", "value", priorityStr, "error", err)
		}
	}
	if priorityInt < 0 || priorityInt > 7 {
		logger.Log.Debug("parsed priority out of bounds. using fallback value", "value", priorityInt)
		priorityInt = defaultSeverity
	}
	severity := severityMap[priorityInt]

	// Prepare metadata fields
	identField := parsedEntry["SYSLOG_IDENTIFIER"]
	logEntry.Metadata = map[string]string{
		"priority":   severity,
		"identifier": identField.First(),
	}

	// Parse message
	msgField := parsedEntry["MESSAGE"]
	logEntry.Text = msgField.First()

	return logEntry, nil
}
