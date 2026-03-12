package memory

import (
	"fmt"
	"testing"

	"github.com/shirou/gopsutil/v4/mem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"agent/internal/collection"
	"agent/internal/metrics"
)

type mockPS struct {
	mock.Mock
}

func (m *mockPS) VirtualMemory() (*mem.VirtualMemoryStat, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mem.VirtualMemoryStat), args.Error(1)
}

func (m *mockPS) SwapMemory() (*mem.SwapMemoryStat, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mem.SwapMemoryStat), args.Error(1)
}

func TestMemoryCollector(t *testing.T) {
	var mps mockPS
	defer mps.AssertExpectations(t)

	vm := &mem.VirtualMemoryStat{
		Total:       16000000000,
		Available:   8000000000,
		Used:        8000000000,
		Free:        4000000000,
		UsedPercent: 50.0,
	}

	sm := &mem.SwapMemoryStat{
		Total:       8000000000,
		Used:        2000000000,
		Free:        6000000000,
		UsedPercent: 25.0,
	}

	mps.On("VirtualMemory").Return(vm, nil).Once()
	mps.On("SwapMemory").Return(sm, nil).Once()

	c := &MemoryCollector{
		ps: &mps,
	}

	dps, err := c.CollectAll()
	require.NoError(t, err)

	assertContainsMetric(t, dps, "mem_total_bytes", 16000000000.0)
	assertContainsMetric(t, dps, "mem_available_bytes", 8000000000.0)
	assertContainsMetric(t, dps, "mem_used_bytes", 8000000000.0)
	assertContainsMetric(t, dps, "mem_free_bytes", 4000000000.0)
	assertContainsMetric(t, dps, "mem_used_ratio", 0.5)

	assertContainsMetric(t, dps, "mem_swap_total_bytes", 8000000000.0)
	assertContainsMetric(t, dps, "mem_swap_used_bytes", 2000000000.0)
	assertContainsMetric(t, dps, "mem_swap_free_bytes", 6000000000.0)
	assertContainsMetric(t, dps, "mem_swap_used_ratio", 0.25)
}

func TestMemoryCollector_Errors(t *testing.T) {
	t.Run("VirtualMemoryError", func(t *testing.T) {
		var mps mockPS
		mps.On("VirtualMemory").Return(nil, fmt.Errorf("vm error")).Once()
		c := &MemoryCollector{ps: &mps}
		_, err := c.CollectAll()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "vm error")
	})

	t.Run("SwapMemoryError", func(t *testing.T) {
		var mps mockPS
		vm := &mem.VirtualMemoryStat{}
		mps.On("VirtualMemory").Return(vm, nil).Once()
		mps.On("SwapMemory").Return(nil, fmt.Errorf("swap error")).Once()
		c := &MemoryCollector{ps: &mps}
		_, err := c.CollectAll()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "swap error")
	})
}

func TestMemoryCollector_Discover(t *testing.T) {
	var mps mockPS
	vm := &mem.VirtualMemoryStat{}
	sm := &mem.SwapMemoryStat{}
	mps.On("VirtualMemory").Return(vm, nil).Once()
	mps.On("SwapMemory").Return(sm, nil).Once()

	c := &MemoryCollector{ps: &mps}
	discovered, err := c.Discover()
	require.NoError(t, err)

	// 5 mem metrics + 4 swap metrics = 9 metrics
	assert.Equal(t, 9, len(discovered))
}

func TestMemoryCollector_CollectFiltering(t *testing.T) {
	var mps mockPS
	vm := &mem.VirtualMemoryStat{Total: 1000}
	sm := &mem.SwapMemoryStat{Total: 500}
	mps.On("VirtualMemory").Return(vm, nil).Once()
	mps.On("SwapMemory").Return(sm, nil).Once()

	c := &MemoryCollector{ps: &mps}
	c.SetIncludedMetrics([]collection.Metric{
		{Name: "mem_total_bytes"},
	})

	dps, err := c.Collect()
	require.NoError(t, err)
	assert.Len(t, dps, 1)
	assert.Equal(t, "mem_total_bytes", dps[0].Name)
}

func assertContainsMetric(t *testing.T, dps []metrics.DataPoint, name string, value float64) {
	for _, dp := range dps {
		if dp.Name == name {
			assert.InDelta(t, value, dp.Value, 0.0001, "Metric %s", name)
			return
		}
	}
	assert.Failf(t, "Metric not found", "Could not find metric %q", name)
}
