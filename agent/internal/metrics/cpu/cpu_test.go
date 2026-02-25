package cpu

import (
	"fmt"
	"testing"

	"github.com/shirou/gopsutil/v4/cpu"

	"agent/internal/collection"
)

type mockPS struct {
	timesFunc func(perCPU bool) ([]cpu.TimesStat, error)
}

func (m *mockPS) CPUTimes(perCPU bool) ([]cpu.TimesStat, error) {
	return m.timesFunc(perCPU)
}

func TestCPUCollector(t *testing.T) {
	c := NewCPUCollector()
	if c.Name() != "cpu" {
		t.Errorf("expected cpu, got %s", c.Name())
	}
}

func TestCPUCollector_Collection(t *testing.T) {
	tests := []struct {
		name      string
		lastStats []cpu.TimesStat
		currStats []cpu.TimesStat
		expected  map[string]float64
	}{
		{
			name: "basic collection",
			lastStats: []cpu.TimesStat{
				{CPU: "cpu0", User: 100, System: 50, Idle: 500},
			},
			currStats: []cpu.TimesStat{
				{CPU: "cpu0", User: 110, System: 55, Idle: 585},
			},
			expected: map[string]float64{
				"cpu_user_ratio":   0.1,
				"cpu_system_ratio": 0.05,
				"cpu_idle_ratio":   0.85,
			},
		},
		{
			name: "with guest adjustment",
			lastStats: []cpu.TimesStat{
				{CPU: "cpu0", User: 100, Nice: 20, Guest: 10, GuestNice: 5},
			},
			currStats: []cpu.TimesStat{
				{CPU: "cpu0", User: 120, Nice: 30, Guest: 15, GuestNice: 7, Idle: 110},
			},
			// dUser=20, dNice=10, dGuest=5, dGuestNice=2, dIdle=110
			// adjUser = 20-5=15, adjNice = 10-2=8
			// total = 15+8+110+5+2 = 140
			expected: map[string]float64{
				"cpu_user_ratio": 15.0 / 140.0,
				"cpu_nice_ratio": 8.0 / 140.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockPS{timesFunc: func(bool) ([]cpu.TimesStat, error) { return tt.currStats, nil }}
			c := &CPUCollector{ps: mock, lastStats: tt.lastStats}

			dps, err := c.CollectAll()
			if err != nil {
				t.Fatalf("CollectAll failed: %v", err)
			}

			for name, expected := range tt.expected {
				found := false
				for _, dp := range dps {
					if dp.Name == name && dp.Labels["cpu"] == "total" {
						found = true
						if dp.Value < expected-0.0001 || dp.Value > expected+0.0001 {
							t.Errorf("%s: expected %f, got %f", name, expected, dp.Value)
						}
					}
				}
				if !found {
					t.Errorf("%s metric not found", name)
				}
			}
		})
	}
}

func TestCPUCollector_Initial(t *testing.T) {
	stats := []cpu.TimesStat{{CPU: "cpu0", User: 100, Idle: 500}}
	calls := 0
	mock := &mockPS{timesFunc: func(bool) ([]cpu.TimesStat, error) {
		calls++
		return stats, nil
	}}
	c := &CPUCollector{ps: mock} // lastStats is nil

	_, _ = c.CollectAll()
	if calls != 2 {
		t.Errorf("expected 2 calls on initial collect, got %d", calls)
	}
}

func TestCPUCollector_Discover(t *testing.T) {
	mock := &mockPS{timesFunc: func(bool) ([]cpu.TimesStat, error) {
		return []cpu.TimesStat{{CPU: "cpu0"}, {CPU: "cpu1"}}, nil
	}}
	c := &CPUCollector{ps: mock}

	m, err := c.Discover()
	if err != nil || len(m) != 30 {
		t.Errorf("Discover failed: err=%v, count=%d", err, len(m))
	}
}

func TestCPUCollector_Filtering(t *testing.T) {
	mock := &mockPS{timesFunc: func(bool) ([]cpu.TimesStat, error) {
		return []cpu.TimesStat{{CPU: "cpu0", User: 110, Idle: 500}}, nil
	}}
	c := &CPUCollector{ps: mock, lastStats: []cpu.TimesStat{{CPU: "cpu0", User: 100, Idle: 400}}}
	c.SetIncludedMetrics([]collection.Metric{{Name: "cpu_user_ratio", Labels: map[string]string{"cpu": "total"}}})

	dps, _ := c.Collect()
	if len(dps) != 1 || dps[0].Name != "cpu_user_ratio" {
		t.Errorf("Filtering failed, got %d metrics", len(dps))
	}
}

func TestCPUCollector_Errors(t *testing.T) {
	mock := &mockPS{timesFunc: func(bool) ([]cpu.TimesStat, error) { return nil, fmt.Errorf("err") }}
	c := &CPUCollector{ps: mock}

	if _, err := c.CollectAll(); err == nil {
		t.Error("expected error")
	}
}
