package nginx

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"agent/internal/collection"
	"agent/internal/logger"
	"agent/internal/metrics"
	"log/slog"
	"io"
)

func init() {
	logger.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
}

type mockPS struct {
	mock.Mock
}

func (m *mockPS) GetStatusPageBody(url string) (string, error) {
	args := m.Called(url)
	return args.String(0), args.Error(1)
}

const nginxStatusBody = `Active connections: 2 
server accepts handled requests
 10 10 20 
Reading: 0 Writing: 1 Waiting: 1 
`

func TestNginxCollector(t *testing.T) {
	var mps mockPS
	defer mps.AssertExpectations(t)

	mps.On("GetStatusPageBody", mock.Anything).Return(nginxStatusBody, nil).Once()

	c := &NginxCollector{
		ps:  &mps,
		url: "http://localhost/nginx_status",
	}

	dps, err := c.CollectAll()
	require.NoError(t, err)

	assertContainsMetric(t, dps, "nginx_connections_active_total", 2.0)
	assertContainsMetric(t, dps, "nginx_requests_total", 20.0)
	assertContainsMetric(t, dps, "nginx_connections_reading_total", 0.0)
	assertContainsMetric(t, dps, "nginx_connections_writing_total", 1.0)
	assertContainsMetric(t, dps, "nginx_connections_waiting_total", 1.0)
	assertContainsMetric(t, dps, "nginx_requests_rate", 0.0) // No previous stats

	// Second collection for rate calculation
	mps.On("GetStatusPageBody", mock.Anything).Return(`Active connections: 3 
server accepts handled requests
 15 15 30 
Reading: 0 Writing: 2 Waiting: 1 
`, nil).Once()

	dps, err = c.CollectAll()
	require.NoError(t, err)
	
	// Manipulate lastStats to ensure a deterministic rate for testing
	c.lastStats.Ts = dps[0].Timestamp - 1000
	c.lastStats.Requests = 20

	mps.On("GetStatusPageBody", mock.Anything).Return(`Active connections: 3 
server accepts handled requests
 15 15 30 
Reading: 0 Writing: 2 Waiting: 1 
`, nil).Once()

	dps, err = c.CollectAll()
	require.NoError(t, err)

	assertContainsMetric(t, dps, "nginx_requests_rate", 10.0)
}

func TestNginxCollector_CounterReset(t *testing.T) {
	var mps mockPS
	c := &NginxCollector{ps: &mps}
	
	// Pre-fill stats
	c.lastStats = &nginxStats{
		Ts:       time.Now().UnixMilli() - 1000,
		Requests: 100,
	}

	// Nginx restarted, requests is now 20
	mps.On("GetStatusPageBody", mock.Anything).Return(`Active connections: 1 
server accepts handled requests
 5 5 20 
Reading: 0 Writing: 1 Waiting: 0 
`, nil).Once()

	dps, err := c.CollectAll()
	require.NoError(t, err)

	// We use a looser tolerance in assertContainsMetric to handle the small time jitter
	// When reset detected, deltaReq = current.Requests = 20
	// deltaT = ~1000ms -> rate = ~20
	assertContainsMetric(t, dps, "nginx_requests_rate", 20.0)
}

func TestNginxLogCollector_Discover(t *testing.T) {
	var mps mockPS
	mps.On("GetStatusPageBody", mock.Anything).Return(nginxStatusBody, nil).Once()

	c := &NginxCollector{ps: &mps}
	discovered, err := c.Discover()
	require.NoError(t, err)

	// 6 nginxMetrics
	assert.Len(t, discovered, 6)
}

func TestNginxCollector_Errors(t *testing.T) {
	t.Run("GetBodyError", func(t *testing.T) {
		var mps mockPS
		mps.On("GetStatusPageBody", mock.Anything).Return("", fmt.Errorf("http error")).Once()
		c := &NginxCollector{ps: &mps}
		dps, err := c.CollectAll()
		require.NoError(t, err) // CollectAll logs and returns nil, nil on error
		assert.Nil(t, dps)
	})

	t.Run("ParseError", func(t *testing.T) {
		var mps mockPS
		mps.On("GetStatusPageBody", mock.Anything).Return("invalid body", nil).Once()
		c := &NginxCollector{ps: &mps}
		dps, err := c.CollectAll()
		require.NoError(t, err)
		assert.Len(t, dps, 6)
		for _, dp := range dps {
			assert.Equal(t, 0.0, dp.Value)
		}
	})
}

func TestNginxCollector_Filtering(t *testing.T) {
	var mps mockPS
	mps.On("GetStatusPageBody", mock.Anything).Return(nginxStatusBody, nil).Once()

	c := &NginxCollector{ps: &mps}
	c.SetIncludedMetrics([]collection.Metric{
		{Name: "nginx_requests_total"},
	})

	dps, err := c.Collect()
	require.NoError(t, err)
	assert.Len(t, dps, 1)
	assert.Equal(t, "nginx_requests_total", dps[0].Name)
}

func assertContainsMetric(t *testing.T, dps []metrics.DataPoint, name string, value float64) {
	for _, dp := range dps {
		if dp.Name == name {
			// Use 0.5 tolerance to handle real-time jitter without nowFunc
			assert.InDelta(t, value, dp.Value, 0.5, "Metric %s", name)
			return
		}
	}
	assert.Failf(t, "Metric not found", "Could not find metric %q", name)
}
