package processes

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCollector_Collect(t *testing.T) {
	collector := NewCollector()

	// First collection
	payload1, err := collector.Collect()
	assert.NoError(t, err)
	assert.NotEmpty(t, payload1)

	// Verify basic structure of the current process
	foundSelf := false
	selfPid := int32(os.Getpid())
	for _, p := range payload1 {
		if p.Pid == selfPid {
			assert.NotEmpty(t, p.Name)
			assert.GreaterOrEqual(t, p.Threads, int32(0))
			foundSelf = true
			break
		}
	}
	assert.True(t, foundSelf, "Should have found the current process")

	// Sleep briefly to simulate time passing for CPU delta calculations
	time.Sleep(100 * time.Millisecond)

	// Second collection (verifies interval-average CPU delta logic doesn't crash/error)
	payload2, err := collector.Collect()
	assert.NoError(t, err)
	assert.NotEmpty(t, payload2)

	// Check that prevStats map has been successfully updated with current processes
	collector.mu.Lock()
	defer collector.mu.Unlock()
	assert.NotEmpty(t, collector.prevStats)
}
