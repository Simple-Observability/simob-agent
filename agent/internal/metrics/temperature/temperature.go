package temperature

import (
	"encoding/json"
	"fmt"

	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/sensors"

	"agent/internal/collection"
	"agent/internal/metrics"
)

type TemperatureCollector struct {
	metrics.BaseCollector
}

func NewTemperatureCollector() *TemperatureCollector {
	return &TemperatureCollector{}
}

func (c *TemperatureCollector) Name() string {
	return "temp"
}

// tempMetrics list the available metrics inside the temperature package
var tempMetrics = []struct {
	name     string
	unit     string
	getValue func(*mem.VirtualMemoryStat) float64
}{
	{"mem_total_bytes", "bytes", func(vm *mem.VirtualMemoryStat) float64 { return float64(vm.Total) }},
}

func (c *TemperatureCollector) Collect() ([]metrics.DataPoint, error) {
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

func (c *TemperatureCollector) CollectAll() ([]metrics.DataPoint, error) {
	return nil, nil
}

func (c *TemperatureCollector) Discover() ([]collection.Metric, error) {
	//var discovered []collection.Metric

	data, err := sensors.SensorsTemperatures()
	if err != nil {
		return nil, fmt.Errorf("failed to discover temperature metrics: %w", err)
	}

	b, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(b))
	return nil, nil
}
