package exporter

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"agent/internal/config"
)

func TestFlusher_SendPayload(t *testing.T) {
	var receivedPayload []MetricPayload
	var receivedAuthHeader string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthHeader = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedPayload)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	cfg := &config.Config{
		APIKey:           "test-api-key",
		MetricsExportUrl: ts.URL,
	}

	f, err := newFlusher(nil, cfg, false)
	require.NoError(t, err)

	payload := []Payload{
		MetricPayload{Name: "test_m1", Value: 1.0},
		MetricPayload{Name: "test_m2", Value: 2.0},
	}

	err = f.sendPayload(ts.URL, payload)
	require.NoError(t, err)

	assert.Equal(t, "test-api-key", receivedAuthHeader)
	require.Len(t, receivedPayload, 2)
	assert.Equal(t, "test_m1", receivedPayload[0].Name)
	assert.Equal(t, "test_m2", receivedPayload[1].Name)
}

func TestFlusher_FlushOnce(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "flusher_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	s, err := newSpool(withDirectory(tempDir))
	require.NoError(t, err)
	defer s.close()

	// Put some metrics into the spool
	now := time.Now().UnixMilli()
	m1 := MetricPayload{Timestamp: strconv.FormatInt(now, 10), Name: "m1", Value: 1.0}
	err = s.append(m1)
	require.NoError(t, err)

	var receivedCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedCount++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	cfg := &config.Config{
		APIKey:           "key",
		MetricsExportUrl: ts.URL,
	}

	f, err := newFlusher(s, cfg, false)
	require.NoError(t, err)

	// flushOnce for metrics - with retries in test because diskqueue is async
	var hasMore bool
	var flushErr error
	for i := 0; i < 40; i++ {
		hasMore, flushErr = f.flushOnce(payloadConfig{name: "metrics", url: ts.URL, unmarshal: unmarshalMetric})
		if flushErr == nil && receivedCount > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	require.NoError(t, flushErr)
	assert.False(t, hasMore)
	assert.Equal(t, 1, receivedCount)

	// flushOnce again - should be empty
	hasMore, flushErr = f.flushOnce(payloadConfig{name: "metrics", url: ts.URL, unmarshal: unmarshalMetric})
	require.NoError(t, flushErr)
	assert.False(t, hasMore)
	assert.Equal(t, 1, receivedCount) // No new request
}

func TestFlusher_DryRun(t *testing.T) {
	cfg := &config.Config{
		APIKey:           "key",
		MetricsExportUrl: "http://invalid-url",
	}

	f, err := newFlusher(nil, cfg, true)
	// dryRun = true
	require.NoError(t, err)

	payload := []Payload{
		MetricPayload{Name: "test_m1", Value: 1.0},
	}

	// Should not fail even if URL is invalid, because it's a dry run
	err = f.sendPayload("http://invalid-url", payload)
	require.NoError(t, err)
}
