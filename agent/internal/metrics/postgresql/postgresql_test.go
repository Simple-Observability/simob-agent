package postgresql

import (
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"agent/internal/logger"
	"agent/internal/metrics"
)

func init() {
	logger.Log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

type mockPostgreSQLClient struct {
	mock.Mock
}

func (m *mockPostgreSQLClient) getDatabaseStats() ([]databaseStat, error) {
	args := m.Called()
	return args.Get(0).([]databaseStat), args.Error(1)
}

func (m *mockPostgreSQLClient) getBgwriterStats() (*bgwriterStats, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*bgwriterStats), args.Error(1)
}

func (m *mockPostgreSQLClient) close() error {
	args := m.Called()
	return args.Error(0)
}

func TestPostgresqlCollector(t *testing.T) {
	mockClient := new(mockPostgreSQLClient)

	dbStatsData := []databaseStat{
		{
			Name: "test_db",
			Stats: dbStats{
				Active:       2,
				XactCommit:   100,
				XactRollback: 5,
				BlksRead:     1000,
				BlksHit:      5000,
				TupReturned:  2000,
				TupFetched:   1500,
				TupInserted:  100,
				TupUpdated:   50,
				TupDeleted:   10,
				Conflicts:    1,
				Deadlocks:    0,
			},
		},
	}

	bgStatsData := &bgwriterStats{
		CheckpointsTimed:  10,
		CheckpointsReq:    2,
		BuffersCheckpoint: 500,
		BuffersClean:      200,
		BuffersBackend:    100,
		BuffersAlloc:      1000,
	}

	mockClient.On("getDatabaseStats").Return(dbStatsData, nil).Once()
	mockClient.On("getBgwriterStats").Return(bgStatsData, nil).Once()

	c := &PostgresqlCollector{
		client:      mockClient,
		lastDBStats: make(map[string]dbStats),
	}

	// First collection
	dps, err := c.CollectAll()
	require.NoError(t, err)

	// Should contain the gauge metric
	assertContainsMetric(t, dps, "postgresql_connections_active", 2.0, "test_db")
	// Should not contain rates on first run
	assertNoMetric(t, dps, "postgresql_transactions_committed_rate")

	// Second collection for rates
	lastTime := c.lastTime
	c.lastTime = lastTime - 1000 // 1 second ago

	dbStatsData2 := []databaseStat{
		{
			Name: "test_db",
			Stats: dbStats{
				Active:       3,
				XactCommit:   110,  // +10
				XactRollback: 6,    // +1
				BlksRead:     1100, // +100
				BlksHit:      5500, // +500
				TupReturned:  2200, // +200
				TupFetched:   1650, // +150
				TupInserted:  110,  // +10
				TupUpdated:   55,   // +5
				TupDeleted:   11,   // +1
				Conflicts:    2,    // +1
				Deadlocks:    1,    // +1
			},
		},
	}

	bgStatsData2 := &bgwriterStats{
		CheckpointsTimed:  11,   // +1
		CheckpointsReq:    3,    // +1
		BuffersCheckpoint: 600,  // +100
		BuffersClean:      250,  // +50
		BuffersBackend:    120,  // +20
		BuffersAlloc:      1100, // +100
	}

	mockClient.On("getDatabaseStats").Return(dbStatsData2, nil).Once()
	mockClient.On("getBgwriterStats").Return(bgStatsData2, nil).Once()

	dps, err = c.CollectAll()
	require.NoError(t, err)

	// Connections Active
	assertContainsMetric(t, dps, "postgresql_connections_active", 3.0, "test_db")

	// DB Rates
	assertContainsMetric(t, dps, "postgresql_transactions_committed_rate", 10.0, "test_db")
	assertContainsMetric(t, dps, "postgresql_transactions_rolled_back_rate", 1.0, "test_db")
	assertContainsMetric(t, dps, "postgresql_blocks_read_rate", 100.0, "test_db")
	assertContainsMetric(t, dps, "postgresql_blocks_hit_rate", 500.0, "test_db")
	assertContainsMetric(t, dps, "postgresql_rows_returned_rate", 200.0, "test_db")
	assertContainsMetric(t, dps, "postgresql_rows_fetched_rate", 150.0, "test_db")
	assertContainsMetric(t, dps, "postgresql_rows_inserted_rate", 10.0, "test_db")
	assertContainsMetric(t, dps, "postgresql_rows_updated_rate", 5.0, "test_db")
	assertContainsMetric(t, dps, "postgresql_rows_deleted_rate", 1.0, "test_db")
	assertContainsMetric(t, dps, "postgresql_conflicts_rate", 1.0, "test_db")
	assertContainsMetric(t, dps, "postgresql_deadlocks_rate", 1.0, "test_db")

	// Bgwriter Rates
	assertContainsMetric(t, dps, "postgresql_checkpoints_timed_rate", 1.0, "")
	assertContainsMetric(t, dps, "postgresql_checkpoints_req_rate", 1.0, "")
	assertContainsMetric(t, dps, "postgresql_buffers_checkpoint_rate", 100.0, "")
	assertContainsMetric(t, dps, "postgresql_buffers_clean_rate", 50.0, "")
	assertContainsMetric(t, dps, "postgresql_buffers_backend_rate", 20.0, "")
	assertContainsMetric(t, dps, "postgresql_buffers_alloc_rate", 100.0, "")

	mockClient.AssertExpectations(t)
}

func assertContainsMetric(t *testing.T, dps []metrics.DataPoint, name string, value float64, dbLabel string) {
	for _, dp := range dps {
		if dp.Name == name {
			if dbLabel != "" && dp.Labels["db"] != dbLabel {
				continue
			}
			assert.InDelta(t, value, dp.Value, 1.0, "Metric %s (db: %s)", name, dbLabel)
			return
		}
	}
	assert.Failf(t, "Metric not found", "Could not find metric %q (db: %s)", name, dbLabel)
}

func assertNoMetric(t *testing.T, dps []metrics.DataPoint, name string) {
	for _, dp := range dps {
		if dp.Name == name {
			assert.Failf(t, "Metric found", "Did not expect metric %q but found it", name)
		}
	}
}
