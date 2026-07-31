package journalctl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"agent/internal/logs"
)

func TestProcessJSONEntry(t *testing.T) {
	c := NewJournalCTLCollector()

	tests := []struct {
		name     string
		input    string
		expected string // Expected text message
		priority string // Expected priority string
		ident    string // Expected syslog identifier
	}{
		{
			name: "basic string message",
			input: `{
				"__REALTIME_TIMESTAMP": "1675204481123456",
				"PRIORITY": "6",
				"SYSLOG_IDENTIFIER": "sshd",
				"MESSAGE": "Accepted publickey for root"
			}`,
			expected: "Accepted publickey for root",
			priority: "info",
			ident:    "sshd",
		},
		{
			name: "message as byte array (non-utf8 representation in json)",
			input: `{
				"__REALTIME_TIMESTAMP": "1675204481123456",
				"PRIORITY": "3",
				"SYSLOG_IDENTIFIER": "kernel",
				"MESSAGE": [65, 99, 99, 101, 112, 116, 101, 100]
			}`,
			expected: "Accepted",
			priority: "error",
			ident:    "kernel",
		},
		{
			name: "missing priority falls back to info",
			input: `{
				"__REALTIME_TIMESTAMP": "1675204481123456",
				"SYSLOG_IDENTIFIER": "test",
				"MESSAGE": "hello world"
			}`,
			expected: "hello world",
			priority: "info",
			ident:    "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, err := c.processJSONEntry([]byte(tt.input))
			require.NoError(t, err)

			assert.Equal(t, tt.expected, entry.Text)
			assert.Equal(t, tt.priority, entry.Metadata["priority"])
			assert.Equal(t, tt.ident, entry.Metadata["identifier"])

			// 1675204481123456 microseconds = 1675204481123 milliseconds
			assert.Equal(t, int64(1675204481123), entry.Timestamp)
		})
	}
}

func TestShouldSkip(t *testing.T) {
	tests := []struct {
		name     string
		ident    string
		expected bool
	}{
		{name: "self identifier skipped", ident: "simob", expected: true},
		{name: "other identifier kept", ident: "sshd", expected: false},
		{name: "empty identifier kept", ident: "", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := logs.LogEntry{Metadata: map[string]string{"identifier": tt.ident}}
			assert.Equal(t, tt.expected, shouldSkip(entry))
		})
	}
}
