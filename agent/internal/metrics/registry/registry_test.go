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

	// Status + cpu + mem = 3
	assert.Len(t, collectors, 3)

	names := make(map[string]bool)
	for _, c := range collectors {
		names[c.Name()] = true
	}

	assert.True(t, names["status"])
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

	// Only status collector should remain
	assert.Len(t, collectors, 1)
	assert.Equal(t, "status", collectors[0].Name())
}
