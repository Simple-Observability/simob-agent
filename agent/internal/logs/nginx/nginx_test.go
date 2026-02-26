package nginx

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNginxLogCollector_ProcessLogLine(t *testing.T) {
	c := NewNginxLogCollector()

	tests := []struct {
		name      string
		logLine   string
		expectErr bool
		expectedT int64 // UnixMilli
	}{
		{
			name:      "Valid log line",
			logLine:   `127.0.0.1 - - [26/Feb/2026:10:00:00 +0000] "GET / HTTP/1.1" 200 612`,
			expectErr: false,
			expectedT: 1772100000000,
		},
		{
			name:      "Valid log line with different timezone",
			logLine:   `127.0.0.1 - - [26/Feb/2026:10:00:00 +0200] "GET / HTTP/1.1" 200 612`,
			expectErr: false,
			expectedT: 1772092800000, // 2 hours earlier in UTC
		},
		{
			name:      "Missing timestamp",
			logLine:   `127.0.0.1 - - "GET / HTTP/1.1" 200 612`,
			expectErr: true,
		},
		{
			name:      "Invalid timestamp format",
			logLine:   `127.0.0.1 - - [26-Feb-2026 10:00:00] "GET / HTTP/1.1" 200 612`,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, err := c.processLogLine(tt.logLine)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, "nginx", entry.Source)
				assert.Equal(t, tt.logLine, entry.Text)
				assert.Equal(t, tt.expectedT, entry.Timestamp)
				assert.NotContains(t, entry.Labels, "timestamp")
			}
		})
	}
}
