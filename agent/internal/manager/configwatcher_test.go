package manager

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"agent/internal/api"
	"agent/internal/collection"
	"agent/internal/config"
	"agent/internal/logger"
)

func init() {
	logger.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestConfigWatcher_ReloadBlocking(t *testing.T) {
	// Setup a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := collection.CollectionConfig{
			LogSources: []collection.LogSource{{Name: "test", Path: "/var/log/test"}},
		}
		// Trigger hash change every time
		cfg.LogSources[0].Name = "test" + time.Now().Format(time.RFC3339Nano)
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(cfg)
		require.NoError(t, err)
	}))
	defer server.Close()

	// Initial config to start with
	initialCfg := &collection.CollectionConfig{}

	// Create client pointing to mock server
	apiClient := api.NewClient(config.Config{
		APIUrl: server.URL,
		APIKey: "test-key",
	}, false)

	// Create a reload channel with buffer size 1
	reloadCh := make(chan bool, 1)

	cw := NewConfigWatcher(apiClient, reloadCh)
	// Set initial hash
	hash, err := initialCfg.Hash()
	require.NoError(t, err)
	cw.initialHash = hash

	// 1st change detection
	cw.checkConfigForChange()
	assert.Len(t, reloadCh, 1)

	// 2nd change detection (should not block)
	done := make(chan bool)
	go func() {
		cw.checkConfigForChange()
		done <- true
	}()

	select {
	case <-done:
		t.Log("Second check completed successfully (no block).")
	case <-time.After(2 * time.Second):
		assert.Fail(t, "checkConfigForChange blocked on second reload")
	}
}
