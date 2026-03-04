package status

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusCollector(t *testing.T) {
	c := NewStatusCollector()
	assert.Equal(t, "status", c.Name())

	dps, err := c.CollectAll()
	require.NoError(t, err)
	require.Len(t, dps, 1)

	dp := dps[0]
	assert.Equal(t, "heartbeat", dp.Name)
	assert.Equal(t, 1.0, dp.Value)
	assert.NotZero(t, dp.Timestamp)
	assert.Empty(t, dp.Labels)
}

func TestStatusCollector_Discover(t *testing.T) {
	c := NewStatusCollector()
	discovered, err := c.Discover()
	require.NoError(t, err)
	assert.Empty(t, discovered)
}
