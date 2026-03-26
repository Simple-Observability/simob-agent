package apache

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockApachePS struct {
	body string
	err  error
}

func (m *mockApachePS) GetStatusPageBody(url string) (string, error) {
	return m.body, m.err
}

func TestApacheCollector_CollectAll(t *testing.T) {
	body := `localhost
ServerVersion: Apache/2.4.58 (Unix)
ServerMPM: event
Server Built: Oct 19 2023 00:00:00
CurrentTime: Thursday, 26-OCt-2023 15:30:20 UTC
RestartTime: Thursday, 26-OCt-2023 15:30:10 UTC
ParentServerConfigGeneration: 1
ParentServerMPMGeneration: 0
ServerUptimeSeconds: 10
ServerUptime: 10 seconds
Load1: 0.05
Load5: 0.10
Load15: 0.15
Total Accesses: 100
Total kBytes: 50
Total Duration: 0
Uptime: 10
ReqPerSec: 10
BytesPerSec: 5120
BytesPerReq: 512
BusyWorkers: 2
IdleWorkers: 8
Scoreboard: _W_R_.......`

	c := NewApacheCollector()
	c.ps = &mockApachePS{body: body}

	// We don't filter in CollectAll, we get all values
	datapoints, err := c.CollectAll()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(datapoints) == 0 {
		t.Fatalf("expected datapoints, got 0")
	}

	expectedMetrics := map[string]float64{
		"apache_accesses_total":                100,
		"apache_requests_rate":                 0,
		"apache_bytes_total":                   50 * 1024,
		"apache_bytes_rate":                    0,
		"apache_uptime_seconds":                10,
		"apache_workers_busy_total":            2,
		"apache_workers_idle_total":            8,
		"apache_scoreboard_waiting_total":      3,
		"apache_scoreboard_sending_total":      1,
		"apache_scoreboard_reading_total":      1,
		"apache_scoreboard_open_total":         7,
		"apache_scoreboard_starting_total":     0,
		"apache_scoreboard_keepalive_total":    0,
		"apache_scoreboard_dnslookup_total":    0,
		"apache_scoreboard_closing_total":      0,
		"apache_scoreboard_logging_total":      0,
		"apache_scoreboard_finishing_total":    0,
		"apache_scoreboard_idle_cleanup_total": 0,
	}

	require.Len(t, datapoints, len(expectedMetrics))

	for _, dp := range datapoints {
		expected, ok := expectedMetrics[dp.Name]
		require.True(t, ok, "unexpected metric %s", dp.Name)
		assert.Equal(t, expected, dp.Value, "metric %s value", dp.Name)
		assert.NotZero(t, dp.Timestamp, "metric %s timestamp", dp.Name)
	}
}

func TestApacheCollector_Discover(t *testing.T) {
	c := NewApacheCollector()
	c.ps = &mockApachePS{body: "Total Accesses: 0"}

	discovered, err := c.Discover()
	require.NoError(t, err)
	require.Len(t, discovered, 18)

	// Verify all returned metrics have type "gauge" and the correct units
	for _, d := range discovered {
		assert.Equal(t, "gauge", d.Type, "metric type for %s", d.Name)
		if d.Name == "apache_uptime_seconds" {
			assert.Equal(t, "s", d.Unit, "expected unit s for %s", d.Name)
		}
	}
}
