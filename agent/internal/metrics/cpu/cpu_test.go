package cpu

import (
	"fmt"
	"testing"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"agent/internal/collection"
	"agent/internal/metrics"
)

type mockPS struct {
	mock.Mock
}

func (m *mockPS) CPUTimes(perCPU bool) ([]cpu.TimesStat, error) {
	args := m.Called(perCPU)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]cpu.TimesStat), args.Error(1)
}

func TestCPUCollector(t *testing.T) {
	var mps mockPS
	defer mps.AssertExpectations(t)

	cts1 := cpu.TimesStat{
		CPU:    "cpu0",
		User:   100.0,
		System: 50.0,
		Idle:   500.0,
	}

	cts2 := cpu.TimesStat{
		CPU:    "cpu0",
		User:   110.0, // +10
		System: 55.0,  // +5
		Idle:   585.0, // +85 (total delta 100)
	}

	// First collection (initializes lastStats)
	mps.On("CPUTimes", true).Return([]cpu.TimesStat{cts1}, nil).Once()
	// Second call in CollectAll (after sleep)
	mps.On("CPUTimes", true).Return([]cpu.TimesStat{cts2}, nil).Once()

	c := &CPUCollector{
		ps: &mps,
	}

	dps, err := c.CollectAll()
	require.NoError(t, err)

	// total labels
	labels := map[string]string{"cpu": "total"}
	assertContainsMetric(t, dps, "cpu_user_ratio", 0.1, labels)
	assertContainsMetric(t, dps, "cpu_system_ratio", 0.05, labels)
	assertContainsMetric(t, dps, "cpu_idle_ratio", 0.85, labels)

	// Per-core labels
	coreLabels := map[string]string{"cpu": "cpu0"}
	assertContainsMetric(t, dps, "cpu_user_ratio", 0.1, coreLabels)

	// Second collection using existing lastStats
	cts3 := cpu.TimesStat{
		CPU:    "cpu0",
		User:   125.0, // +15
		System: 60.0,  // +5
		Idle:   665.0, // +80 (total delta 100)
	}
	mps.On("CPUTimes", true).Return([]cpu.TimesStat{cts3}, nil).Once()

	dps, err = c.CollectAll()
	require.NoError(t, err)

	assertContainsMetric(t, dps, "cpu_user_ratio", 0.15, labels)
	assertContainsMetric(t, dps, "cpu_system_ratio", 0.05, labels)
	assertContainsMetric(t, dps, "cpu_idle_ratio", 0.8, labels)
}

func TestCPUCollector_GuestAdjustment(t *testing.T) {
	var mps mockPS
	c := &CPUCollector{ps: &mps}

	cts1 := cpu.TimesStat{
		CPU:       "cpu0",
		User:      100.0,
		Nice:      20.0,
		Guest:     10.0,
		GuestNice: 5.0,
	}
	cts2 := cpu.TimesStat{
		CPU:       "cpu0",
		User:      120.0, // deltaUser = 20, deltaGuest = 5 -> adjUser = 15
		Nice:      30.0,  // deltaNice = 10, deltaGuestNice = 2 -> adjNice = 8
		Idle:      110.0, // deltaIdle = 110
		Guest:     15.0,  // deltaGuest = 5
		GuestNice: 7.0,   // deltaGuestNice = 2
	}
	// Total = 15 + 8 + 110 + 5 + 2 = 140

	mps.On("CPUTimes", true).Return([]cpu.TimesStat{cts2}, nil).Once()
	c.lastStats = []cpu.TimesStat{cts1}

	dps, err := c.CollectAll()
	require.NoError(t, err)

	labels := map[string]string{"cpu": "total"}
	assertContainsMetric(t, dps, "cpu_user_ratio", 15.0/140.0, labels)
	assertContainsMetric(t, dps, "cpu_nice_ratio", 8.0/140.0, labels)
	assertContainsMetric(t, dps, "cpu_guest_ratio", 5.0/140.0, labels)
}

func TestCPUCollector_Errors(t *testing.T) {
	var mps mockPS
	c := &CPUCollector{ps: &mps}

	mps.On("CPUTimes", true).Return(nil, fmt.Errorf("error")).Once()
	_, err := c.CollectAll()
	require.Error(t, err)

	mps.On("CPUTimes", true).Return(nil, fmt.Errorf("error")).Once()
	_, err = c.Discover()
	require.Error(t, err)
}

func TestCPUCollector_Discover(t *testing.T) {
	var mps mockPS
	c := &CPUCollector{ps: &mps}

	cts := []cpu.TimesStat{{CPU: "cpu0"}, {CPU: "cpu1"}}
	mps.On("CPUTimes", true).Return(cts, nil).Once()

	discovered, err := c.Discover()
	require.NoError(t, err)

	// 10 fields per core (2 cores) + 10 fields for "total" = 30 metrics
	assert.Equal(t, 30, len(discovered))
}

func TestCPUCollector_CollectFiltering(t *testing.T) {
	var mps mockPS
	c := &CPUCollector{ps: &mps}

	cts1 := cpu.TimesStat{CPU: "cpu0", User: 100.0, Idle: 500.0}
	cts2 := cpu.TimesStat{CPU: "cpu0", User: 110.0, Idle: 590.0}

	c.lastStats = []cpu.TimesStat{cts1}
	mps.On("CPUTimes", true).Return([]cpu.TimesStat{cts2}, nil).Once()

	// Filter to only include cpu_user_ratio for total
	c.SetIncludedMetrics([]collection.Metric{
		{
			Name:   "cpu_user_ratio",
			Labels: map[string]string{"cpu": "total"},
		},
	})

	dps, err := c.Collect()
	require.NoError(t, err)
	assert.Len(t, dps, 1)
	assert.Equal(t, "cpu_user_ratio", dps[0].Name)
	assert.Equal(t, "total", dps[0].Labels["cpu"])
}

func assertContainsMetric(t *testing.T, dps []metrics.DataPoint, name string, value float64, labels map[string]string) {
	for _, dp := range dps {
		if dp.Name == name && labelsEqual(dp.Labels, labels) {
			assert.InDelta(t, value, dp.Value, 0.0001, "Metric %s with labels %v", name, labels)
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
