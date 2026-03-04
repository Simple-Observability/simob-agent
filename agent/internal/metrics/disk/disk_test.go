package disk

import (
	"fmt"
	"testing"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"agent/internal/metrics"
)

type mockPS struct {
	mock.Mock
}

func (m *mockPS) Partitions(all bool) ([]disk.PartitionStat, error) {
	args := m.Called(all)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]disk.PartitionStat), args.Error(1)
}

func (m *mockPS) Usage(path string) (*disk.UsageStat, error) {
	args := m.Called(path)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*disk.UsageStat), args.Error(1)
}

func (m *mockPS) IOCounters(names ...string) (map[string]disk.IOCountersStat, error) {
	args := m.Called(names)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]disk.IOCountersStat), args.Error(1)
}

func TestDiskCollector(t *testing.T) {
	var mps mockPS
	defer mps.AssertExpectations(t)

	partitions := []disk.PartitionStat{
		{Device: "/dev/sda1", Mountpoint: "/", Fstype: "ext4"},
	}

	usage := &disk.UsageStat{
		Path:              "/",
		Total:             1000000,
		Free:              400000,
		Used:              600000,
		UsedPercent:       60.0,
		InodesTotal:       1000,
		InodesFree:        400,
		InodesUsed:        600,
		InodesUsedPercent: 60.0,
	}

	io1 := map[string]disk.IOCountersStat{
		"sda1": {
			Name:       "sda1",
			ReadCount:  100,
			WriteCount: 50,
			ReadBytes:  1000,
			WriteBytes: 500,
			IoTime:     100,
			ReadTime:   50,
			WriteTime:  50,
		},
	}

	io2 := map[string]disk.IOCountersStat{
		"sda1": {
			Name:       "sda1",
			ReadCount:  110,  // +10
			WriteCount: 60,   // +10
			ReadBytes:  1100, // +100
			WriteBytes: 650,  // +150
			IoTime:     150,  // +50
			ReadTime:   60,   // +10
			WriteTime:  70,   // +20
		},
	}

	mps.On("Partitions", false).Return(partitions, nil).Twice()
	mps.On("Usage", "/").Return(usage, nil).Twice()
	mps.On("IOCounters", mock.Anything).Return(io1, nil).Once()
	mps.On("IOCounters", mock.Anything).Return(io2, nil).Once()

	c := &DiskCollector{
		ps:        &mps,
		lastStats: make(map[string]disk.IOCountersStat),
	}

	// First collection (initializes lastStats)
	dps, err := c.CollectAll()
	require.NoError(t, err)

	labels := map[string]string{"device": "/dev/sda1", "mountpoint": "/"}
	assertContainsMetric(t, dps, "disk_total_bytes", 1000000.0, labels)
	assertContainsMetric(t, dps, "disk_used_ratio", 0.6, labels)
	// IO metrics should NOT be present in first run as deltaT/lastStats are not ready
	assertNoMetric(t, dps, "disk_read_rate", labels)

	// Update lastTime to simulate time passing
	c.lastTime = c.lastTime - 1000 // 1 second ago

	// Second collection
	dps, err = c.CollectAll()
	require.NoError(t, err)

	// deltaT should be 1000ms (1s)
	// deltaReadCount = 10 -> rate = 10 / 1s * 1000ms/s = 10.0
	assertContainsMetric(t, dps, "disk_read_rate", 10.0, labels)
	assertContainsMetric(t, dps, "disk_write_rate", 10.0, labels)
	assertContainsMetric(t, dps, "disk_read_bps", 100.0, labels)
	assertContainsMetric(t, dps, "disk_write_bps", 150.0, labels)
	// deltaIoTime = 50, deltaT = 1000 -> ratio = 0.05
	assertContainsMetric(t, dps, "disk_busy_ratio", 0.05, labels)
	// totalTime = 10+20=30, totalOps = 10+10=20 -> avg = 1.5
	assertContainsMetric(t, dps, "disk_avg_request_ms", 1.5, labels)
}

func TestDiskCollector_UniquePartitions(t *testing.T) {
	var mps mockPS
	partitions := []disk.PartitionStat{
		{Device: "/dev/sda1", Mountpoint: "/", Opts: []string{"rw"}},
		{Device: "/dev/sda1", Mountpoint: "/mnt/bind", Opts: []string{"rw", "bind"}}, // Bind mount, skip
		{Device: "/dev/sda1", Mountpoint: "/other", Opts: []string{"rw"}},           // Same device, skip
		{Device: "/dev/sdb1", Mountpoint: "/data", Opts: []string{"rw"}},            // New device, keep
	}

	mps.On("Partitions", false).Return(partitions, nil).Once()

	c := &DiskCollector{ps: &mps}
	unique, err := c.getUniquePrimaryPartitions()
	require.NoError(t, err)

	assert.Len(t, unique, 2)
	assert.Equal(t, "/", unique[0].Mountpoint)
	assert.Equal(t, "/data", unique[1].Mountpoint)
}

func TestDiskCollector_Discover(t *testing.T) {
	var mps mockPS
	partitions := []disk.PartitionStat{
		{Device: "/dev/sda1", Mountpoint: "/", Fstype: "ext4"},
	}
	mps.On("Partitions", false).Return(partitions, nil).Once()

	c := &DiskCollector{ps: &mps}
	discovered, err := c.Discover()
	require.NoError(t, err)

	// 8 diskMetrics + 6 diskIOMetrics = 14
	assert.Len(t, discovered, 14)
}

func TestDiskCollector_Errors(t *testing.T) {
	t.Run("PartitionsError", func(t *testing.T) {
		var mps mockPS
		mps.On("Partitions", false).Return(nil, fmt.Errorf("error")).Once()
		c := &DiskCollector{ps: &mps}
		_, err := c.CollectAll()
		require.Error(t, err)
	})

	t.Run("IOCountersError", func(t *testing.T) {
		var mps mockPS
		mps.On("Partitions", false).Return([]disk.PartitionStat{}, nil).Once()
		mps.On("IOCounters", mock.Anything).Return(nil, fmt.Errorf("error")).Once()
		c := &DiskCollector{ps: &mps}
		_, err := c.CollectAll()
		require.Error(t, err)
	})
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

func assertNoMetric(t *testing.T, dps []metrics.DataPoint, name string, labels map[string]string) {
	for _, dp := range dps {
		if dp.Name == name && labelsEqual(dp.Labels, labels) {
			assert.Failf(t, "Metric found", "Did not expect to find metric %q with labels %v", name, labels)
		}
	}
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
