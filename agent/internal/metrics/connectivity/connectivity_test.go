package connectivity

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

type mockDialer struct {
	mock.Mock
}

func (m *mockDialer) DialTimeout(address string, timeout time.Duration) (time.Duration, error) {
	args := m.Called(address, timeout)
	duration, _ := args.Get(0).(time.Duration)
	return duration, args.Error(1)
}

func newTestCollector(m *mockDialer) *ConnectivityCollector {
	return &ConnectivityCollector{
		dialer:  m,
		targets: []string{"1.1.1.1:443", "8.8.8.8:443", "9.9.9.9:443"},
	}
}

func TestConnectivityCollector_AllTargetsUp(t *testing.T) {
	var md mockDialer
	defer md.AssertExpectations(t)

	md.On("DialTimeout", "1.1.1.1:443", dialTimeout).Return(20*time.Millisecond, nil).Once()
	md.On("DialTimeout", "8.8.8.8:443", dialTimeout).Return(35*time.Millisecond, nil).Once()
	md.On("DialTimeout", "9.9.9.9:443", dialTimeout).Return(10*time.Millisecond, nil).Once()

	c := newTestCollector(&md)
	dps, err := c.CollectAll()
	require.NoError(t, err)
	require.Len(t, dps, 2)

	up, latency := findDataPoints(t, dps)
	assert.Equal(t, "connectivity", up.Name)
	assert.Equal(t, 1.0, up.Value)
	assert.Equal(t, "connectivity_latency_ms", latency.Name)
	assert.InDelta(t, 10.0, latency.Value, 0.001)
	assert.Empty(t, up.Labels)
	assert.NotZero(t, up.Timestamp)
}

func TestConnectivityCollector_PartialFailure(t *testing.T) {
	var md mockDialer
	defer md.AssertExpectations(t)

	md.On("DialTimeout", "1.1.1.1:443", dialTimeout).Return(time.Duration(0), fmt.Errorf("timeout")).Once()
	md.On("DialTimeout", "8.8.8.8:443", dialTimeout).Return(15*time.Millisecond, nil).Once()
	md.On("DialTimeout", "9.9.9.9:443", dialTimeout).Return(40*time.Millisecond, nil).Once()

	c := newTestCollector(&md)
	dps, err := c.CollectAll()
	require.NoError(t, err)

	up, latency := findDataPoints(t, dps)
	assert.Equal(t, 1.0, up.Value)
	assert.InDelta(t, 15.0, latency.Value, 0.001)
}

func TestConnectivityCollector_AllTargetsDown(t *testing.T) {
	var md mockDialer
	defer md.AssertExpectations(t)

	for _, target := range []string{"1.1.1.1:443", "8.8.8.8:443", "9.9.9.9:443"} {
		md.On("DialTimeout", target, dialTimeout).Return(time.Duration(0), fmt.Errorf("no route to host")).Once()
	}

	c := newTestCollector(&md)
	dps, err := c.CollectAll()
	require.NoError(t, err)
	require.Len(t, dps, 1)

	assert.Equal(t, "connectivity", dps[0].Name)
	assert.Equal(t, 0.0, dps[0].Value)
}

func TestConnectivityCollector_CollectWithoutLatencyConfigured(t *testing.T) {
	var md mockDialer
	defer md.AssertExpectations(t)

	md.On("DialTimeout", "1.1.1.1:443", dialTimeout).Return(10*time.Millisecond, nil).Once()
	md.On("DialTimeout", "8.8.8.8:443", dialTimeout).Return(10*time.Millisecond, nil).Once()
	md.On("DialTimeout", "9.9.9.9:443", dialTimeout).Return(10*time.Millisecond, nil).Once()

	c := newTestCollector(&md)

	// No included metrics configured: only the always-on status signal
	// is emitted, the latency metric is filtered out.
	dps, err := c.Collect()
	require.NoError(t, err)
	require.Len(t, dps, 1)
	assert.Equal(t, "connectivity", dps[0].Name)
}

func TestConnectivityCollector_CollectWithLatencyConfigured(t *testing.T) {
	var md mockDialer
	defer md.AssertExpectations(t)

	md.On("DialTimeout", "1.1.1.1:443", dialTimeout).Return(10*time.Millisecond, nil).Once()
	md.On("DialTimeout", "8.8.8.8:443", dialTimeout).Return(10*time.Millisecond, nil).Once()
	md.On("DialTimeout", "9.9.9.9:443", dialTimeout).Return(10*time.Millisecond, nil).Once()

	c := newTestCollector(&md)
	c.SetIncludedMetrics([]collection.Metric{
		{Name: "connectivity_latency_ms", Labels: map[string]string{}},
	})

	// With latency selected in the config, both datapoints are emitted.
	dps, err := c.Collect()
	require.NoError(t, err)
	assert.Len(t, dps, 2)
}

func TestConnectivityCollector_CollectDownWithLatencyConfigured(t *testing.T) {
	var md mockDialer
	defer md.AssertExpectations(t)

	for _, target := range []string{"1.1.1.1:443", "8.8.8.8:443", "9.9.9.9:443"} {
		md.On("DialTimeout", target, dialTimeout).Return(time.Duration(0), fmt.Errorf("no route to host")).Once()
	}

	c := newTestCollector(&md)
	c.SetIncludedMetrics([]collection.Metric{
		{Name: "connectivity_latency_ms", Labels: map[string]string{}},
	})

	// Offline: only the status signal is emitted (value 0), no latency point.
	dps, err := c.Collect()
	require.NoError(t, err)
	require.Len(t, dps, 1)
	assert.Equal(t, "connectivity", dps[0].Name)
	assert.Equal(t, 0.0, dps[0].Value)
}

func TestConnectivityCollector_Discover(t *testing.T) {
	c := NewConnectivityCollector()
	assert.Equal(t, "connectivity", c.Name())

	// Only the latency metric is advertised as available: the
	// `connectivity` status signal is always-on and not configurable.
	discovered, err := c.Discover()
	require.NoError(t, err)
	require.Len(t, discovered, 1)
	assert.Equal(t, "connectivity_latency_ms", discovered[0].Name)
	assert.Equal(t, "gauge", discovered[0].Type)
}

func findDataPoints(t *testing.T, dps []metrics.DataPoint) (up metrics.DataPoint, latency metrics.DataPoint) {
	t.Helper()
	for _, dp := range dps {
		switch dp.Name {
		case "connectivity":
			up = dp
		case "connectivity_latency_ms":
			latency = dp
		}
	}
	require.NotZero(t, up.Timestamp, "connectivity datapoint missing")
	require.NotZero(t, latency.Timestamp, "connectivity_latency_ms datapoint missing")
	return up, latency
}
