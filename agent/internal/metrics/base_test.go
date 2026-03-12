package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"agent/internal/collection"
)

func TestBaseCollector_IsIncluded(t *testing.T) {
	bc := &BaseCollector{}
	
	included := []collection.Metric{
		{Name: "cpu_user_ratio", Labels: map[string]string{"cpu": "total"}},
		{Name: "mem_used_bytes", Labels: map[string]string{}},
	}
	bc.SetIncludedMetrics(included)

	tests := []struct {
		name     string
		metric   string
		labels   map[string]string
		expected bool
	}{
		{
			name:     "Exact match with labels",
			metric:   "cpu_user_ratio",
			labels:   map[string]string{"cpu": "total"},
			expected: true,
		},
		{
			name:     "Exact match without labels",
			metric:   "mem_used_bytes",
			labels:   map[string]string{},
			expected: true,
		},
		{
			name:     "Name match but labels mismatch",
			metric:   "cpu_user_ratio",
			labels:   map[string]string{"cpu": "cpu0"},
			expected: false,
		},
		{
			name:     "Name mismatch",
			metric:   "disk_used_bytes",
			labels:   map[string]string{},
			expected: false,
		},
		{
			name:     "Empty labels vs nil labels",
			metric:   "mem_used_bytes",
			labels:   nil,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, bc.IsIncluded(tt.metric, tt.labels))
		})
	}
}

func TestLabelsEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        map[string]string
		b        map[string]string
		expected bool
	}{
		{"Both nil", nil, nil, true},
		{"Both empty", map[string]string{}, map[string]string{}, true},
		{"Nil and empty", nil, map[string]string{}, true},
		{"Same content", map[string]string{"k": "v"}, map[string]string{"k": "v"}, true},
		{"Different keys", map[string]string{"k1": "v"}, map[string]string{"k2": "v"}, false},
		{"Different values", map[string]string{"k": "v1"}, map[string]string{"k": "v2"}, false},
		{"Different lengths", map[string]string{"k": "v"}, map[string]string{"k": "v", "k2": "v2"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, labelsEqual(tt.a, tt.b))
		})
	}
}
