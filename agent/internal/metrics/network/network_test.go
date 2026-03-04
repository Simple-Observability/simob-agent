package network

import (
	"fmt"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/net"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"agent/internal/collection"
	"agent/internal/metrics"
)

type mockPS struct {
	mock.Mock
}

func (m *mockPS) IOCounters(pernic bool) ([]net.IOCountersStat, error) {
	args := m.Called(pernic)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]net.IOCountersStat), args.Error(1)
}

func TestNetworkCollector(t *testing.T) {
	var mps mockPS
	defer mps.AssertExpectations(t)

	io1 := []net.IOCountersStat{
		{
			Name:      "eth0",
			BytesSent: 1000,
			BytesRecv: 2000,
		},
	}

	io2 := []net.IOCountersStat{
		{
			Name:      "eth0",
			BytesSent: 2000, // +1000
			BytesRecv: 4000, // +2000
		},
	}

	mps.On("IOCounters", true).Return(io1, nil).Once()
	mps.On("IOCounters", true).Return(io2, nil).Once()

	c := &NetworkCollector{
		ps: &mps,
	}

	// First collection (initializes lastStats)
	dps, err := c.CollectAll()
	require.NoError(t, err)
	assert.Empty(t, dps)

	// Update lastTime to simulate time passing (1 second ago)
	c.lastTime = time.Now().Add(-1 * time.Second)

	// Second collection
	dps, err = c.CollectAll()
	require.NoError(t, err)

	labels := map[string]string{"interface": "eth0"}
	// deltaBytesSent = 1000, deltaT approx 1s -> value approx 1000.0
	assertContainsMetric(t, dps, "net_bytes_sent_bps", 1000.0, labels)
	assertContainsMetric(t, dps, "net_bytes_recv_bps", 2000.0, labels)
}

func TestNetworkCollector_Discover(t *testing.T) {
	var mps mockPS
	ioStats := []net.IOCountersStat{
		{Name: "eth0"},
		{Name: "lo"},
	}
	mps.On("IOCounters", true).Return(ioStats, nil).Once()

	c := &NetworkCollector{ps: &mps}
	discovered, err := c.Discover()
	require.NoError(t, err)

	// 2 interfaces * 8 metrics = 16
	assert.Len(t, discovered, 16)
}

func TestNetworkCollector_Errors(t *testing.T) {
	var mps mockPS
	mps.On("IOCounters", true).Return(nil, fmt.Errorf("network error")).Once()

	c := &NetworkCollector{ps: &mps}
	_, err := c.CollectAll()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network error")
}

func TestNetworkCollector_Filtering(t *testing.T) {
	var mps mockPS
	io1 := []net.IOCountersStat{{Name: "eth0", BytesSent: 100}}
	io2 := []net.IOCountersStat{{Name: "eth0", BytesSent: 200}}
	mps.On("IOCounters", true).Return(io1, nil).Once()
	mps.On("IOCounters", true).Return(io2, nil).Once()

	c := &NetworkCollector{ps: &mps}
	c.SetIncludedMetrics([]collection.Metric{
		{Name: "net_bytes_sent_bps", Labels: map[string]string{"interface": "eth0"}},
	})

	// First call init
	_, _ = c.Collect()
	c.lastTime = time.Now().Add(-1 * time.Second)

	// Second call collect
	dps, err := c.Collect()
	require.NoError(t, err)
	assert.Len(t, dps, 1)
	assert.Equal(t, "net_bytes_sent_bps", dps[0].Name)
}

func assertContainsMetric(t *testing.T, dps []metrics.DataPoint, name string, value float64, labels map[string]string) {
	for _, dp := range dps {
		if dp.Name == name && labelsEqual(dp.Labels, labels) {
			// Use a small delta for consistency
			assert.InDelta(t, value, dp.Value, 0.001, "Metric %s with labels %v", name, labels)
			return
		}
	}
	assert.Failf(t, "Metric not found", "Could not find metric %q with labels %v", name, labels)
}

func labelsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
