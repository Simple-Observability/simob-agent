package phpfpm

import (
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"agent/internal/collection"
	"agent/internal/logger"
	"agent/internal/metrics"
)

func init() {
	logger.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
}

type mockClient struct {
	mock.Mock
}

func (m *mockClient) GetStats() (*FPMStatus, error) {
	args := m.Called()
	stats, _ := args.Get(0).(*FPMStatus)
	return stats, args.Error(1)
}

func TestPHPFPMCollector(t *testing.T) {
	var client mockClient
	defer client.AssertExpectations(t)

	timestamp := time.Unix(0, 0)
	c := &Collector{
		client: &client,
		now:    func() time.Time { return timestamp },
	}

	client.On("GetStats").Return(&FPMStatus{
		ListenQueue:        1,
		MaxListenQueue:     3,
		ListenQueueLen:     128,
		IdleProcesses:      5,
		ActiveProcesses:    2,
		TotalProcesses:     7,
		MaxActiveProcesses: 4,
		AcceptedConn:       100,
		MaxChildrenReached: 1,
		SlowRequests:       2,
	}, nil).Once()

	dps, err := c.CollectAll()
	require.NoError(t, err)
	require.Len(t, dps, len(metricDefinitions))

	assertContainsMetric(t, dps, "phpfpm_listen_queue_total", 1)
	assertContainsMetric(t, dps, "phpfpm_max_listen_queue_total", 3)
	assertContainsMetric(t, dps, "phpfpm_listen_queue_length_total", 128)
	assertContainsMetric(t, dps, "phpfpm_idle_processes_total", 5)
	assertContainsMetric(t, dps, "phpfpm_active_processes_total", 2)
	assertContainsMetric(t, dps, "phpfpm_processes_total", 7)
	assertContainsMetric(t, dps, "phpfpm_max_active_processes_total", 4)
	assertContainsMetric(t, dps, "phpfpm_accepted_connections_rate", 0)
	assertContainsMetric(t, dps, "phpfpm_max_children_reached_total", 1)
	assertContainsMetric(t, dps, "phpfpm_slow_requests_rate", 0)

	timestamp = timestamp.Add(time.Second)
	client.On("GetStats").Return(&FPMStatus{
		ListenQueue:        2,
		MaxListenQueue:     4,
		ListenQueueLen:     128,
		IdleProcesses:      4,
		ActiveProcesses:    3,
		TotalProcesses:     7,
		MaxActiveProcesses: 5,
		AcceptedConn:       130,
		MaxChildrenReached: 2,
		SlowRequests:       5,
	}, nil).Once()

	dps, err = c.CollectAll()
	require.NoError(t, err)

	assertContainsMetric(t, dps, "phpfpm_accepted_connections_rate", 30)
	assertContainsMetric(t, dps, "phpfpm_slow_requests_rate", 3)
	assertContainsMetric(t, dps, "phpfpm_max_children_reached_total", 2)
}

func TestPHPFPMCollector_CounterReset(t *testing.T) {
	var client mockClient
	defer client.AssertExpectations(t)

	timestamp := time.Unix(0, 0)
	c := &Collector{
		client: &client,
		now:    func() time.Time { return timestamp },
		lastStats: &FPMStatus{
			Timestamp:    timestamp.Add(-time.Second).UnixMilli(),
			AcceptedConn: 200,
			SlowRequests: 9,
		},
	}

	client.On("GetStats").Return(&FPMStatus{
		AcceptedConn: 20,
		SlowRequests: 2,
	}, nil).Once()

	dps, err := c.CollectAll()
	require.NoError(t, err)

	assertContainsMetric(t, dps, "phpfpm_accepted_connections_rate", 20)
	assertContainsMetric(t, dps, "phpfpm_slow_requests_rate", 2)
}

func TestPHPFPMCollector_Discover(t *testing.T) {
	var client mockClient
	defer client.AssertExpectations(t)

	c := &Collector{
		client: &client,
		now:    time.Now,
	}

	client.On("GetStats").Return(&FPMStatus{}, nil).Once()

	discovered, err := c.Discover()
	require.NoError(t, err)
	require.Len(t, discovered, len(metricDefinitions))
	assert.Equal(t, "phpfpm_listen_queue_total", discovered[0].Name)
	assert.Equal(t, "phpfpm_max_children_reached_total", discovered[8].Name)
	assert.Equal(t, "counter", discovered[8].Type)
}

func TestPHPFPMCollector_Filtering(t *testing.T) {
	var client mockClient
	defer client.AssertExpectations(t)

	c := &Collector{
		client: &client,
		now:    time.Now,
	}
	c.SetIncludedMetrics([]collection.Metric{
		{Name: "phpfpm_active_processes_total"},
		{Name: "phpfpm_slow_requests_rate"},
	})

	client.On("GetStats").Return(&FPMStatus{
		ActiveProcesses: 6,
		SlowRequests:    1,
	}, nil).Once()

	dps, err := c.Collect()
	require.NoError(t, err)
	require.Len(t, dps, 2)
	assertContainsMetric(t, dps, "phpfpm_active_processes_total", 6)
	assertContainsMetric(t, dps, "phpfpm_slow_requests_rate", 0)
}

func TestPHPFPMCollector_Errors(t *testing.T) {
	t.Run("GetStatsError", func(t *testing.T) {
		var client mockClient
		defer client.AssertExpectations(t)

		c := &Collector{client: &client, now: time.Now}
		client.On("GetStats").Return((*FPMStatus)(nil), fmt.Errorf("dial error")).Once()

		dps, err := c.CollectAll()
		require.NoError(t, err)
		assert.Nil(t, dps)
	})
}

func TestParseFastCGIHTTPResponse(t *testing.T) {
	body, statusCode, err := parseFastCGIHTTPResponse([]byte("Status: 200 OK\r\nContent-Type: application/json\r\n\r\n{\"pool\":\"www\"}"))
	require.NoError(t, err)
	assert.Equal(t, 200, statusCode)
	assert.JSONEq(t, `{"pool":"www"}`, string(body))
}

func assertContainsMetric(t *testing.T, dps []metrics.DataPoint, name string, value float64) {
	for _, dp := range dps {
		if dp.Name == name {
			assert.Equal(t, value, dp.Value, "Metric %s", name)
			return
		}
	}

	assert.Failf(t, "metric not found", "Could not find metric %q", name)
}
