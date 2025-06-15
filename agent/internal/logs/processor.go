package logs

import (
	"fmt"
	"regexp"
	"time"
)

func processLogLine(rawLog RawLogLine, regex string) (LogEntry, error) {
	entry := LogEntry{
		Source: rawLog.Source,
		Text:   rawLog.Text,
		Labels: make(map[string]string),
	}

	// Match labels
	re := regexp.MustCompile(regex)
	matches := re.FindStringSubmatch(rawLog.Text)
	if matches == nil {
		return LogEntry{}, fmt.Errorf("can't match any label in logline")
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
			return LogEntry{}, fmt.Errorf("failed to parse timestamp: %v", err)
		}
		entry.Timestamp = timestamp.UnixMilli()
		delete(entry.Labels, "timestamp")
	}

	return entry, nil
}
