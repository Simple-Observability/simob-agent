package memory

import (
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v4/mem"

	"agent/internal/metrics"
)

type MemoryCollector struct {
}

func NewMemoryCollector() *MemoryCollector {
	return &MemoryCollector{}
}

func (c *MemoryCollector) Name() string {
	return "memory"
}

// memMetrics list the available metrics inside the memory package
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

func (c *MemoryCollector) Collect() ([]metrics.DataPoint, error) {
	timestamp := time.Now().UnixMilli()

	vm, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("failed to get virtual memory info: %w", err)
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

	return results, nil
}

func (c *MemoryCollector) Discover() ([]metrics.Metric, error) {
	_, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("failed to discover memory metrics: %w", err)
	}

	var discovered []metrics.Metric

	for _, m := range memMetrics {
		discovered = append(discovered, metrics.Metric{
			Name:   m.name,
			Type:   "gauge",
			Unit:   m.unit,
			Labels: map[string]string{},
		})
	}

	return discovered, nil
}
