//go:build linux
// +build linux

package journalctl

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/coreos/go-systemd/sdjournal"

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
	journal *sdjournal.Journal
	cancel  context.CancelFunc
	wg      sync.WaitGroup
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
	// Try to open journal to check if it's available
	journal, err := sdjournal.NewJournal()
	if err != nil {
		return []collection.LogSource{}
	}
	defer journal.Close()

	return []collection.LogSource{
		{
			Name: c.name,
			Path: "",
		},
	}
}

func (c *JournalCTLCollector) Start(ctx context.Context, out chan<- logs.LogEntry) error {
	if c.journal != nil {
		return fmt.Errorf("journalctl collector already running")
	}

	journal, err := sdjournal.NewJournal()
	if err != nil {
		return fmt.Errorf("failed to open system journal: %w", err)
	}

	if err := journal.SeekTail(); err != nil {
		journal.Close()
		return fmt.Errorf("failed to seek to the end of the journal: %w", err)
	}

	c.journal = journal

	// Create a child context so the collector can be stopped independently via
	// c.cancel while still respecting cancellation from the parent context.
	collectorCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	c.wg.Add(1)
	go c.readJournal(collectorCtx, out)

	return nil
}

func (c *JournalCTLCollector) Stop() error {
	if c.cancel != nil {
		c.cancel()
	}

	// Wait for the goroutine to finish
	c.wg.Wait()

	if c.journal != nil {
		if err := c.journal.Close(); err != nil {
			return fmt.Errorf("failed to close journal: %w", err)
		}
		c.journal = nil
	}
	return nil
}

func (c *JournalCTLCollector) readJournal(ctx context.Context, out chan<- logs.LogEntry) {
	defer c.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			fmt.Println("Waiting on new journal entry...")
			r := c.journal.Wait(sdjournal.IndefiniteWait)
			if r == sdjournal.SD_JOURNAL_APPEND {
				// TODO Isn't it better to return the result here and do the rest instead of embedding the logic deep into the call stack?
				if err := c.processNewEntries(out); err != nil {
					logger.Log.Error("can't process new entries", "error", err)
					continue
				}
			}
		}
	}
}

func (c *JournalCTLCollector) processNewEntries(out chan<- logs.LogEntry) error {
	for {
		n, err := c.journal.Next()
		if err != nil {
			return fmt.Errorf("error reading next entry: %w", err)
		}

		if n == 0 {
			// Reached the current end of the journal
			break
		}

		entry, err := c.journal.GetEntry()
		if err != nil {
			logger.Log.Error("can't get journal entry", "error", err)
			continue
		}

		logEntry, err := c.processJournalEntry(entry)
		if err != nil {
			logger.Log.Error("can't process journal entry", "error", err)
			continue
		}

		out <- logEntry
	}

	return nil
}

func (c *JournalCTLCollector) processJournalEntry(entry *sdjournal.JournalEntry) (logs.LogEntry, error) {
	logEntry := logs.LogEntry{
		Source: c.name,
		Labels: make(map[string]string),
	}

	// Parse timestamp
	tsMicro := entry.RealtimeTimestamp
	logEntry.Timestamp = int64(tsMicro / 1000)

	// Parse priority/severity
	priorityStr := entry.Fields[sdjournal.SD_JOURNAL_FIELD_PRIORITY]
	priorityInt, err := strconv.Atoi(priorityStr)
	if err != nil {
		logger.Log.Error("can't process priority. using fallback value", "value", priorityStr, "error", err)
		priorityInt = 6
	}
	if priorityInt < 0 || priorityInt > 7 {
		logger.Log.Error("parsed priority out of bounds. using fallback value", "value", priorityInt, "error", err)
		priorityInt = 6
	}
	severity := severityMap[priorityInt]
	logEntry.Labels["priority"] = severity

	logEntry.Text = entry.Fields[sdjournal.SD_JOURNAL_FIELD_MESSAGE]

	/*
		Metadata fields, commented out for now but will integrated later

		var metaFields = map[string]string{
			"pid": entry.Fields[sdjournal.SD_JOURNAL_FIELD_PID],
			"identifier": entry.Fields[sdjournal.SD_JOURNAL_FIELD_SYSLOG_IDENTIFIER],
			"priority": severity,
			"message": entry.Fields[sdjournal.SD_JOURNAL_FIELD_MESSAGE],
			"transport": entry.Fields[sdjournal.SD_JOURNAL_FIELD_TRANSPORT],
		}
	*/
	return logEntry, nil
}
