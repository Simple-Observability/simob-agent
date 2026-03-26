package memcached

import (
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"agent/internal/logger"
	"agent/internal/metrics"
)

func init() {
	logger.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
}

type mockPS struct {
	mock.Mock
}

func (m *mockPS) GetStats(address string) (string, error) {
	args := m.Called(address)
	return args.String(0), args.Error(1)
}

func TestParseMemcachedStats(t *testing.T) {
	body := `STAT pid 5619
STAT uptime 11
STAT time 1644765868
STAT version 1.6.14_5_ge03751b
STAT libevent 2.1.11-stable
STAT pointer_size 64
STAT rusage_user 0.080905
STAT rusage_system 0.059330
STAT max_connections 1024
STAT curr_connections 2
STAT total_connections 3
STAT rejected_connections 0
STAT connection_structures 3
STAT response_obj_oom 0
STAT response_obj_count 1
STAT response_obj_bytes 16384
STAT read_buf_count 2
STAT read_buf_bytes 32768
STAT read_buf_bytes_free 0
STAT read_buf_oom 0
STAT reserved_fds 20
STAT cmd_get 0
STAT cmd_set 0
STAT cmd_flush 0
STAT cmd_touch 0
STAT cmd_meta 0
STAT get_hits 0
STAT get_misses 0
STAT get_expired 0
STAT get_flushed 0
STAT delete_misses 0
STAT delete_hits 0
STAT incr_misses 0
STAT incr_hits 0
STAT decr_misses 0
STAT decr_hits 0
STAT cas_misses 0
STAT cas_hits 0
STAT cas_badval 0
STAT touch_hits 0
STAT touch_misses 0
STAT store_too_large 0
STAT store_no_memory 0
STAT auth_cmds 0
STAT auth_errors 0
STAT bytes_read 6
STAT bytes_written 0
STAT limit_maxbytes 67108864
STAT accepting_conns 1
STAT listen_disabled_num 0
STAT time_in_listen_disabled_us 0
STAT threads 4
STAT conn_yields 0
STAT hash_power_level 16
STAT hash_bytes 524288
STAT hash_is_expanding 0
STAT slab_reassign_rescues 0
STAT slab_reassign_chunk_rescues 0
STAT slab_reassign_evictions_nomem 0
STAT slab_reassign_inline_reclaim 0
STAT slab_reassign_busy_items 0
STAT slab_reassign_busy_deletes 0
STAT slab_reassign_running 0
STAT slabs_moved 0
STAT lru_crawler_running 0
STAT lru_crawler_starts 1
STAT lru_maintainer_juggles 60
STAT malloc_fails 0
STAT log_worker_dropped 0
STAT log_worker_written 0
STAT log_watcher_skipped 0
STAT log_watcher_sent 0
STAT log_watchers 0
STAT extstore_compact_lost 3287
STAT extstore_compact_rescues 47014
STAT extstore_compact_resc_cold 0
STAT extstore_compact_resc_old 0
STAT extstore_compact_skipped 0
STAT extstore_page_allocs 30047
STAT extstore_page_evictions 25315
STAT extstore_page_reclaims 29247
STAT extstore_pages_free 0
STAT extstore_pages_used 800
STAT extstore_objects_evicted 1243091
STAT extstore_objects_read 938410
STAT extstore_objects_written 1487003
STAT extstore_objects_used 39319
STAT extstore_bytes_evicted 1638804587744
STAT extstore_bytes_written 1951205770118
STAT extstore_bytes_read 1249921752566
STAT extstore_bytes_used 51316205305
STAT extstore_bytes_fragmented 2370885895
STAT extstore_limit_maxbytes 53687091200
STAT extstore_io_queue 0
STAT unexpected_napi_ids 0
STAT round_robin_fallback 0
STAT bytes 0
STAT curr_items 0
STAT total_items 0
STAT slab_global_page_pool 0
STAT expired_unfetched 0
STAT evicted_unfetched 0
STAT evicted_active 0
STAT evictions 0
STAT reclaimed 0
STAT crawler_reclaimed 0
STAT crawler_items_checked 0
STAT lrutail_reflocked 0
STAT moves_to_cold 0
STAT moves_to_warm 0
STAT moves_within_lru 0
STAT direct_reclaims 0
STAT lru_bumps_dropped 0
END
`
	stats := parseMemcachedStats(body)

	expected := map[string]float64{
		"uptime":            11,
		"curr_connections":  2,
		"total_connections": 3,
		"cmd_get":           0,
		"cmd_set":           0,
		"get_hits":          0,
		"get_misses":        0,
		"bytes_read":        6,
		"bytes_written":     0,
		"limit_maxbytes":    67108864,
		"curr_items":        0,
		"total_items":       0,
		"bytes":             0,
	}

	for k, v := range expected {
		assert.Equal(t, v, stats[k], "Metric %s", k)
	}
}

func TestMemcachedCollector_CollectAll(t *testing.T) {
	var mps mockPS
	defer mps.AssertExpectations(t)

	// First collection
	body1 := "STAT uptime 100\nSTAT cmd_get 50\nSTAT curr_items 50\nEND\n"
	mps.On("GetStats", mock.Anything).Return(body1, nil).Once()

	mc := &MemcachedCollector{
		ps:      &mps,
		address: "127.0.0.1:11211",
	}

	dps1, err := mc.CollectAll()
	require.NoError(t, err)

	// First collection now includes rates with 0 value (Nginx style)
	assertContainsMetric(t, dps1, "memcached_uptime_seconds", 100.0)
	assertContainsMetric(t, dps1, "memcached_items_current_total", 50.0)
	assertContainsMetric(t, dps1, "memcached_get_rate", 0.0)

	// Manually set lastStats Ts to 1 second ago for deterministic rate
	mc.lastStats.Ts = dps1[0].Timestamp - 1000

	// Second collection
	body2 := "STAT uptime 101\nSTAT cmd_get 60\nSTAT curr_items 50\nEND\n"
	mps.On("GetStats", mock.Anything).Return(body2, nil).Once()

	dps2, err := mc.CollectAll()
	require.NoError(t, err)

	// Rate should be (60-50) / 1s = 10
	assertContainsMetric(t, dps2, "memcached_get_rate", 10.0)
	// total is no longer reported
	for _, dp := range dps2 {
		assert.NotEqual(t, "memcached_get_total", dp.Name)
	}
}

func TestMemcachedCollector_Discover(t *testing.T) {
	var mps mockPS
	defer mps.AssertExpectations(t)

	mps.On("GetStats", mock.Anything).Return("STAT version 1.6.17\nEND\n", nil).Once()

	mc := &MemcachedCollector{ps: &mps, address: "127.0.0.1:11211"}
	discovered, err := mc.Discover()
	require.NoError(t, err)

	assert.Len(t, discovered, len(memcachedMetrics))

	found := false
	for _, m := range discovered {
		if m.Name == "memcached_uptime_seconds" {
			found = true
			break
		}
	}
	assert.True(t, found, "memcached_uptime_seconds not discovered")
}

func TestMemcachedCollector_Errors(t *testing.T) {
	t.Run("GetStatsError", func(t *testing.T) {
		var mps mockPS
		mps.On("GetStats", mock.Anything).Return("", fmt.Errorf("connection error")).Once()
		mc := &MemcachedCollector{ps: &mps, address: "127.0.0.1:11211"}
		dps, err := mc.CollectAll()
		require.NoError(t, err)
		assert.Nil(t, dps)
	})
}

func assertContainsMetric(t *testing.T, dps []metrics.DataPoint, name string, value float64) {
	for _, dp := range dps {
		if dp.Name == name {
			assert.Equal(t, value, dp.Value, "Metric %s", name)
			return
		}
	}
	assert.Failf(t, "Metric not found", "Could not find metric %q", name)
}
