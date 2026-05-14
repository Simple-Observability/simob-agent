package caddy

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"agent/internal/collection"
	"agent/internal/logger"
	"agent/internal/metrics"

	dto "github.com/prometheus/client_model/go"
	"io"
	"log/slog"
)

func init() {
	logger.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
}

type mockPS struct {
	mock.Mock
}

func (m *mockPS) GetMetrics(url string) (map[string]*dto.MetricFamily, error) {
	args := m.Called(url)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]*dto.MetricFamily), args.Error(1)
}

const caddyMetricsResponse = `
# HELP caddy_http_requests_total The total number of HTTP requests.
# TYPE caddy_http_requests_total counter
caddy_http_requests_total 10
# HELP caddy_http_request_errors_total The total number of HTTP request errors.
# TYPE caddy_http_request_errors_total counter
caddy_http_request_errors_total 2
# HELP caddy_http_response_size_bytes_sum The total number of bytes sent in responses.
# TYPE caddy_http_response_size_bytes_sum counter
caddy_http_response_size_bytes_sum 1024
# HELP caddy_http_request_duration_seconds_sum The total duration of HTTP requests in seconds.
# TYPE caddy_http_request_duration_seconds_sum counter
caddy_http_request_duration_seconds_sum 0.5
# HELP process_resident_memory_bytes Resident memory size in bytes.
# TYPE process_resident_memory_bytes gauge
process_resident_memory_bytes 50000000
# HELP process_cpu_seconds_total Total user and system CPU time spent in seconds.
# TYPE process_cpu_seconds_total counter
process_cpu_seconds_total 1.5
`

func TestCaddyCollector(t *testing.T) {
	var mps mockPS
	defer mps.AssertExpectations(t)

	families, err := metrics.ParsePrometheus(strings.NewReader(caddyMetricsResponse))
	require.NoError(t, err)

	mps.On("GetMetrics", mock.Anything).Return(families, nil).Once()

	c := &CaddyCollector{
		ps:  &mps,
		url: "http://localhost:2019/metrics",
	}

	dps, err := c.CollectAll()
	require.NoError(t, err)

	assertContainsMetric(t, dps, "caddy_http_requests_total", 10.0)
	assertContainsMetric(t, dps, "caddy_http_errors_total", 2.0)
	assertContainsMetric(t, dps, "caddy_http_response_bytes", 1024.0)
	assertContainsMetric(t, dps, "caddy_http_request_duration_ms", 500.0)
	assertContainsMetric(t, dps, "caddy_http_requests_rate", 0.0) // No previous stats

	// Second collection for rate calculation
	caddyResponse2 := strings.Replace(caddyMetricsResponse, "caddy_http_requests_total 10", "caddy_http_requests_total 20", 1)
	families2, _ := metrics.ParsePrometheus(strings.NewReader(caddyResponse2))

	mps.On("GetMetrics", mock.Anything).Return(families2, nil).Once()

	dps, err = c.CollectAll()
	require.NoError(t, err)

	// Manipulate lastStats to ensure a deterministic rate for testing
	c.lastStats.Ts = dps[0].Timestamp - 1000
	c.lastStats.Requests = 10

	mps.On("GetMetrics", mock.Anything).Return(families2, nil).Once()

	dps, err = c.CollectAll()
	require.NoError(t, err)

	assertContainsMetric(t, dps, "caddy_http_requests_rate", 10.0)
}

func TestCaddyCollector_Discover(t *testing.T) {
	var mps mockPS
	families, _ := metrics.ParsePrometheus(strings.NewReader(caddyMetricsResponse))
	mps.On("GetMetrics", mock.Anything).Return(families, nil).Once()

	c := &CaddyCollector{ps: &mps}
	discovered, err := c.Discover()
	require.NoError(t, err)

	// 7 caddyMetrics
	assert.Len(t, discovered, 7)
}

func TestCaddyCollector_Filtering(t *testing.T) {
	var mps mockPS
	families, _ := metrics.ParsePrometheus(strings.NewReader(caddyMetricsResponse))
	mps.On("GetMetrics", mock.Anything).Return(families, nil).Once()

	c := &CaddyCollector{ps: &mps}
	c.SetIncludedMetrics([]collection.Metric{
		{Name: "caddy_http_requests_total"},
	})

	dps, err := c.Collect()
	require.NoError(t, err)
	assert.Len(t, dps, 1)
	assert.Equal(t, "caddy_http_requests_total", dps[0].Name)
}

func assertContainsMetric(t *testing.T, dps []metrics.DataPoint, name string, value float64) {
	for _, dp := range dps {
		if dp.Name == name {
			assert.InDelta(t, value, dp.Value, 0.5, "Metric %s", name)
			return
		}
	}
	assert.Failf(t, "Metric not found", "Could not find metric %q", name)
}
