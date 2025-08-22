package nginx

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"time"

	"agent/internal/collection"
	"agent/internal/logs"
)

type NginxLogCollector struct {
	name    string
	pattern string
	runner  *logs.TailRunner
}

func NewNginxLogCollector() *NginxLogCollector {
	return &NginxLogCollector{
		name:    "nginx",
		pattern: "/var/log/nginx/*.log",
	}
}

func (c *NginxLogCollector) Name() string {
	return c.name
}

func (c *NginxLogCollector) Discover() []collection.LogSource {
	sources := []collection.LogSource{}
	files, _ := filepath.Glob(c.pattern)
	if len(files) > 0 {
		sources = append(sources, collection.LogSource{Name: c.name, Path: c.pattern})
	}
	return sources
}

func (c *NginxLogCollector) Start(ctx context.Context, out chan<- logs.LogEntry) error {
	// Initialize the runner on the first start
	if c.runner == nil {
		runner, err := logs.NewTailRunner(c.pattern, c.processLogLine)
		if err != nil {
			return err
		}
		c.runner = runner
	}
	return c.runner.Start(ctx, out)
}

func (c *NginxLogCollector) Stop() error {
	if c.runner == nil {
		return nil
	}
	return c.runner.Stop()
}

func (c *NginxLogCollector) processLogLine(logLine string) (logs.LogEntry, error) {
	entry := logs.LogEntry{
		Source: c.name,
		Text:   logLine,
		Labels: make(map[string]string),
	}

	// Match labels
	regex := `\[(?P<timestamp>\d{2}/\w{3}/\d{4}:\d{2}:\d{2}:\d{2} [+-]\d{4})\]`
	re := regexp.MustCompile(regex)
	matches := re.FindStringSubmatch(logLine)
	if matches == nil {
		return logs.LogEntry{}, fmt.Errorf("can't match any label in logline")
	}

	// Extract named capture groups directly
	for i, name := range re.SubexpNames() {
		if i != 0 && name != "" && i < len(matches) {
			entry.Labels[name] = matches[i]
		}
	}

	// Parse the timestamp into time.Time
	timestampStr, ok := entry.Labels["timestamp"]
	if ok {
		layout := "02/Jan/2006:15:04:05 -0700"
		timestamp, err := time.Parse(layout, timestampStr)
		if err != nil {
			return logs.LogEntry{}, fmt.Errorf("failed to parse timestamp: %v", err)
		}
		entry.Timestamp = timestamp.UnixMilli()
		delete(entry.Labels, "timestamp")
	}

	return entry, nil
}
