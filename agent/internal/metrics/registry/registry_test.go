package registry

import (
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	"agent/internal/collection"
	"agent/internal/logger"
)

func init() {
	logger.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestBuildCollectors_FilteredConfig(t *testing.T) {
	cfg := &collection.CollectionConfig{
		Metrics: []collection.Metric{
			{Name: "cpu_user_ratio"},
			{Name: "mem_used_bytes"},
		},
	}

	collectors := BuildCollectors(cfg)

	// Status + connectivity + cpu + mem = 4
	assert.Len(t, collectors, 4)

	names := make(map[string]bool)
	for _, c := range collectors {
		names[c.Name()] = true
	}

	assert.True(t, names["status"])
	assert.True(t, names["connectivity"])
	assert.True(t, names["cpu"])
	assert.True(t, names["mem"])
	assert.False(t, names["disk"])
	assert.False(t, names["net"])
	assert.False(t, names["nginx"])
}

func TestBuildCollectors_NoMatch(t *testing.T) {
	cfg := &collection.CollectionConfig{
		Metrics: []collection.Metric{
			{Name: "nonexistent_metric"},
		},
	}

	collectors := BuildCollectors(cfg)

	// Only always-on collectors should remain
	assert.Len(t, collectors, 2)
	names := make(map[string]bool)
	for _, c := range collectors {
		names[c.Name()] = true
	}
	assert.True(t, names["status"])
	assert.True(t, names["connectivity"])
}

func TestBuildCollectors_AlwaysOnWithOptInMetrics(t *testing.T) {
	cfg := &collection.CollectionConfig{
		Metrics: []collection.Metric{
			{Name: "connectivity_latency_ms"},
			{Name: "cpu_user_ratio"},
		},
	}

	collectors := BuildCollectors(cfg)

	// Status + connectivity + cpu, and exactly ONE connectivity collector.
	assert.Len(t, collectors, 3)
	count := 0
	for _, c := range collectors {
		if c.Name() == "connectivity" {
			count++
		}
	}
	assert.Equal(t, 1, count)
}
