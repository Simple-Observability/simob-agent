package exporter

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpool(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "spool_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	s, err := newSpool(withDirectory(tempDir), withSyncEvery(1))
	require.NoError(t, err)
	defer s.close()

	now := time.Now().UnixMilli()
	metric := MetricPayload{Timestamp: strconv.FormatInt(now, 10), Name: "test_metric", Value: 42.0}
	log := LogPayload{Timestamp: strconv.FormatInt(now, 10), Message: "test_log"}

	// Test append
	err = s.append(metric)
	require.NoError(t, err)

	err = s.append(log)
	require.NoError(t, err)

	// Test getBatch for metrics
	metrics, hasMore, err := s.getBatch(metricsQueueName, unmarshalMetric)
	require.NoError(t, err)
	assert.False(t, hasMore)
	require.Len(t, metrics, 1)
	assert.Equal(t, metric.Name, metrics[0].(MetricPayload).Name)

	// Test getBatch for logs
	logs, hasMore, err := s.getBatch(logsQueueName, unmarshalLog)
	require.NoError(t, err)
	assert.False(t, hasMore)
	require.Len(t, logs, 1)
	assert.Equal(t, log.Message, logs[0].(LogPayload).Message)
}

func TestSpoolStaleEntries(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "spool_stale_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	s, err := newSpool(withDirectory(tempDir), withSyncEvery(1))
	require.NoError(t, err)
	defer s.close()

	// Create a stale entry (older than maxAge which is 24h)
	staleTime := time.Now().Add(-25 * time.Hour).UnixMilli()
	freshTime := time.Now().UnixMilli()

	staleMetric := MetricPayload{Timestamp: strconv.FormatInt(staleTime, 10), Name: "stale_metric", Value: 1.0}
	freshMetric := MetricPayload{Timestamp: strconv.FormatInt(freshTime, 10), Name: "fresh_metric", Value: 2.0}

	err = s.append(staleMetric)
	require.NoError(t, err)
	err = s.append(freshMetric)
	require.NoError(t, err)

	// getBatch should skip the stale one
	metrics, _, err := s.getBatch(metricsQueueName, unmarshalMetric)
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Equal(t, "fresh_metric", metrics[0].(MetricPayload).Name)
}

func TestSpoolBatchSize(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "spool_batch_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	s, err := newSpool(withDirectory(tempDir), withSyncEvery(1))
	require.NoError(t, err)
	defer s.close()

	now := time.Now().UnixMilli()
	// Test pagination by adding 150 items. maxBatchSize is 100.
	for i := 0; i < 150; i++ {
		m := MetricPayload{Timestamp: strconv.FormatInt(now, 10), Name: "metric_" + strconv.Itoa(i)}
		err = s.append(m)
		require.NoError(t, err)
	}

	// Wait for diskqueue to sync
	time.Sleep(500 * time.Millisecond)

	// First batch should have 100 items (maxBatchSize)
	metrics1, hasMore1, err := s.getBatch(metricsQueueName, unmarshalMetric)
	require.NoError(t, err)
	assert.Len(t, metrics1, 100)
	assert.True(t, hasMore1)

	// Second batch should have the remaining 50 items
	metrics2, hasMore2, err := s.getBatch(metricsQueueName, unmarshalMetric)
	require.NoError(t, err)
	assert.Len(t, metrics2, 50)
	assert.False(t, hasMore2)
}
