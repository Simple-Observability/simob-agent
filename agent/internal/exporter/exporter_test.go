package exporter

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"agent/internal/logger"
)

func TestExporter_ExportMetric(t *testing.T) {
	logger.Init(true)

	tempDir, err := os.MkdirTemp("", "exporter_metric_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	s, err := newSpool(withDirectory(tempDir), withSyncEvery(1))
	require.NoError(t, err)
	defer s.close()

	e := &Exporter{spool: s}

	now := time.Now().UnixMilli()
	ts := strconv.FormatInt(now, 10)
	metrics := []MetricPayload{
		{Timestamp: ts, Name: "test_m", Value: 1.0},
	}

	err = e.ExportMetric(metrics)
	require.NoError(t, err)

	// Verify it's in the spool
	spooled, _, err := s.getBatch(metricsQueueName, unmarshalMetric)
	require.NoError(t, err)
	assert.Len(t, spooled, 1)
	assert.Equal(t, "test_m", spooled[0].(MetricPayload).Name)
}

func TestExporter_ExportLog(t *testing.T) {
	logger.Init(true)

	tempDir, err := os.MkdirTemp("", "exporter_log_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	s, err := newSpool(withDirectory(tempDir), withSyncEvery(1))
	require.NoError(t, err)
	defer s.close()

	e := &Exporter{spool: s}

	now := time.Now().UnixMilli()
	ts := strconv.FormatInt(now, 10)
	logs := []LogPayload{
		{Timestamp: ts, Message: "test_l"},
	}

	err = e.ExportLog(logs)
	require.NoError(t, err)

	// Verify it's in the spool
	spooled, _, err := s.getBatch(logsQueueName, unmarshalLog)
	require.NoError(t, err)
	assert.Len(t, spooled, 1)
	assert.Equal(t, "test_l", spooled[0].(LogPayload).Message)
}
