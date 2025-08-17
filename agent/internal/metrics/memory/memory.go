package memory

import (
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v4/mem"

	"agent/internal/collection"
	"agent/internal/metrics"
)

type MemoryCollector struct {
	metrics.BaseCollector
}

func NewMemoryCollector() *MemoryCollector {
	return &MemoryCollector{}
}

func (c *MemoryCollector) Name() string {
	return "mem"
}

// memMetrics list the available metrics inside the memory package (virtual memory)
var memMetrics = []struct {
	name     string
	unit     string
	getValue func(*mem.VirtualMemoryStat) float64
}{
	{"mem_total_bytes", "bytes", func(vm *mem.VirtualMemoryStat) float64 { return float64(vm.Total) }},
	{"mem_available_bytes", "bytes", func(vm *mem.VirtualMemoryStat) float64 { return float64(vm.Available) }},
	{"mem_used_bytes", "bytes", func(vm *mem.VirtualMemoryStat) float64 { return float64(vm.Used) }},
	{"mem_free_bytes", "bytes", func(vm *mem.VirtualMemoryStat) float64 { return float64(vm.Free) }},
	{"mem_used_ratio", "%", func(vm *mem.VirtualMemoryStat) float64 { return vm.UsedPercent / 100 }},
}

// swapMetrics list the available metrics inside the memory package (swap)
var swapMetrics = []struct {
	name     string
	unit     string
	getValue func(*mem.SwapMemoryStat) float64
}{
	{"mem_swap_total_bytes", "bytes", func(sm *mem.SwapMemoryStat) float64 { return float64(sm.Total) }},
	{"mem_swap_used_bytes", "bytes", func(sm *mem.SwapMemoryStat) float64 { return float64(sm.Used) }},
	{"mem_swap_free_bytes", "bytes", func(sm *mem.SwapMemoryStat) float64 { return float64(sm.Free) }},
	{"mem_swap_used_ratio", "%", func(sm *mem.SwapMemoryStat) float64 { return sm.UsedPercent / 100 }},
}

func (c *MemoryCollector) Collect() ([]metrics.DataPoint, error) {
	all, err := c.CollectAll()
	if err != nil {
		return nil, err
	}
	var included []metrics.DataPoint
	for _, dp := range all {
		if c.IsIncluded(dp.Name, dp.Labels) {
			included = append(included, dp)
		}
	}
	return included, nil
}

func (c *MemoryCollector) CollectAll() ([]metrics.DataPoint, error) {
	timestamp := time.Now().UnixMilli()

	vm, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("failed to get virtual memory info: %w", err)
	}
	sm, err := mem.SwapMemory()
	if err != nil {
		return nil, fmt.Errorf("failed to get swap memory info: %w", err)
	}

	var results []metrics.DataPoint
	for _, m := range memMetrics {
		results = append(results, metrics.DataPoint{
			Name:      m.name,
			Timestamp: timestamp,
			Value:     m.getValue(vm),
			Labels:    map[string]string{},
		})
	}
	for _, m := range swapMetrics {
		results = append(results, metrics.DataPoint{
			Name:      m.name,
			Timestamp: timestamp,
			Value:     m.getValue(sm),
			Labels:    map[string]string{},
		})
	}
	return results, nil
}

func (c *MemoryCollector) Discover() ([]collection.Metric, error) {
	var discovered []collection.Metric
	_, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("failed to discover memory metrics: %w", err)
	}
	for _, m := range memMetrics {
		discovered = append(discovered, collection.Metric{
			Name:   m.name,
			Type:   "gauge",
			Unit:   m.unit,
			Labels: map[string]string{},
		})
	}
	_, err = mem.SwapMemory()
	if err != nil {
		return nil, fmt.Errorf("failed to discover swap metrics: %w", err)
	}
	for _, m := range swapMetrics {
		discovered = append(discovered, collection.Metric{
			Name:   m.name,
			Type:   "gauge",
			Unit:   m.unit,
			Labels: map[string]string{},
		})
	}

	return discovered, nil
}
