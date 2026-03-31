package postgresql

import (
	"database/sql"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"agent/internal/collection"
	"agent/internal/logger"
	"agent/internal/metrics"
)

type dbMetric struct {
	name   string
	unit   string
	getVal func(current, previous *dbStats, deltaT float64) float64
}

type bgwriterMetric struct {
	name   string
	unit   string
	getVal func(current, previous *bgwriterStats, deltaT float64) float64
}

func computeRate(current, previous float64, deltaT float64) float64 {
	if deltaT <= 0 {
		return 0
	}
	delta := current - previous
	if delta < 0 {
		delta = 0 // Counter reset
	}
	return delta / deltaT
}

func getDBRate(extract func(*dbStats) float64) func(current, previous *dbStats, deltaT float64) float64 {
	return func(current, previous *dbStats, deltaT float64) float64 {
		if previous == nil {
			return 0
		}
		return computeRate(extract(current), extract(previous), deltaT)
	}
}

func getBgwriterRate(extract func(*bgwriterStats) float64) func(current, previous *bgwriterStats, deltaT float64) float64 {
	return func(current, previous *bgwriterStats, deltaT float64) float64 {
		if previous == nil {
			return 0
		}
		return computeRate(extract(current), extract(previous), deltaT)
	}
}

var dbMetrics = []dbMetric{
	{"postgresql_connections_active", "no", func(c, p *dbStats, _ float64) float64 { return c.Active }},
	{"postgresql_transactions_committed_rate", "rate", getDBRate(func(s *dbStats) float64 { return s.XactCommit })},
	{"postgresql_transactions_rolled_back_rate", "rate", getDBRate(func(s *dbStats) float64 { return s.XactRollback })},
	{"postgresql_blocks_read_rate", "rate", getDBRate(func(s *dbStats) float64 { return s.BlksRead })},
	{"postgresql_blocks_hit_rate", "rate", getDBRate(func(s *dbStats) float64 { return s.BlksHit })},
	{"postgresql_rows_returned_rate", "rate", getDBRate(func(s *dbStats) float64 { return s.TupReturned })},
	{"postgresql_rows_fetched_rate", "rate", getDBRate(func(s *dbStats) float64 { return s.TupFetched })},
	{"postgresql_rows_inserted_rate", "rate", getDBRate(func(s *dbStats) float64 { return s.TupInserted })},
	{"postgresql_rows_updated_rate", "rate", getDBRate(func(s *dbStats) float64 { return s.TupUpdated })},
	{"postgresql_rows_deleted_rate", "rate", getDBRate(func(s *dbStats) float64 { return s.TupDeleted })},
	{"postgresql_conflicts_rate", "rate", getDBRate(func(s *dbStats) float64 { return s.Conflicts })},
	{"postgresql_deadlocks_rate", "rate", getDBRate(func(s *dbStats) float64 { return s.Deadlocks })},
}

var bgwriterMetrics = []bgwriterMetric{
	{"postgresql_checkpoints_timed_rate", "rate", getBgwriterRate(func(s *bgwriterStats) float64 { return s.CheckpointsTimed })},
	{"postgresql_checkpoints_req_rate", "rate", getBgwriterRate(func(s *bgwriterStats) float64 { return s.CheckpointsReq })},
	{"postgresql_buffers_checkpoint_rate", "rate", getBgwriterRate(func(s *bgwriterStats) float64 { return s.BuffersCheckpoint })},
	{"postgresql_buffers_clean_rate", "rate", getBgwriterRate(func(s *bgwriterStats) float64 { return s.BuffersClean })},
	{"postgresql_buffers_backend_rate", "rate", getBgwriterRate(func(s *bgwriterStats) float64 { return s.BuffersBackend })},
	{"postgresql_buffers_alloc_rate", "rate", getBgwriterRate(func(s *bgwriterStats) float64 { return s.BuffersAlloc })},
}

type PostgresqlCollector struct {
	metrics.BaseCollector
	client            postgreSQLClient
	lastDBStats       map[string]dbStats
	lastBgwriterStats *bgwriterStats
	lastTime          int64
	connStr           string
}

func NewPostgresqlCollector() *PostgresqlCollector {
	return &PostgresqlCollector{
		connStr:     "postgres://postgres@localhost:5432/postgres?sslmode=disable",
		lastDBStats: make(map[string]dbStats),
	}
}

func (c *PostgresqlCollector) Name() string {
	return "postgresql"
}

func (c *PostgresqlCollector) connect() error {
	if c.client != nil {
		return nil
	}
	db, err := sql.Open("pgx", c.connStr)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	c.client = &realPostgreSQLClient{db: db}
	return nil
}

func (c *PostgresqlCollector) Collect() ([]metrics.DataPoint, error) {
	all, err := c.CollectAll()
	if err != nil {
		return nil, err
	}
	var included []metrics.DataPoint
	for _, dp := range all {
		if c.IsIncluded(dp.Name, dp.Labels) {
			included = append(included, dp)
		}
	}
	return included, nil
}

func (c *PostgresqlCollector) CollectAll() ([]metrics.DataPoint, error) {
	if err := c.connect(); err != nil {
		logger.Log.Debug("Failed to connect to postgres", "error", err)
		return nil, nil
	}

	timestamp := time.Now().UnixMilli()
	deltaT := float64(timestamp-c.lastTime) / 1000.0

	var dps []metrics.DataPoint

	// Collect Database Stats
	dbStatsList, err := c.client.getDatabaseStats()
	if err != nil {
		logger.Log.Debug("Failed to get database stats", "error", err)
	} else {
		for _, ds := range dbStatsList {
			name := ds.Name
			s := ds.Stats
			labels := map[string]string{"db": name}

			var lastPtr *dbStats
			if last, ok := c.lastDBStats[name]; ok && c.lastTime > 0 && deltaT > 0 {
				lastPtr = &last
			}

			for _, m := range dbMetrics {
				val := m.getVal(&s, lastPtr, deltaT)
				// Don't report 0 rates on first run
				if m.unit == "rate" && lastPtr == nil {
					continue
				}
				dps = append(dps, metrics.DataPoint{
					Name:      m.name,
					Timestamp: timestamp,
					Value:     val,
					Labels:    labels,
				})
			}
			c.lastDBStats[name] = s
		}
	}

	// Collect Bgwriter Stats
	b, err := c.client.getBgwriterStats()
	if err != nil {
		logger.Log.Debug("Failed to get bgwriter stats", "error", err)
	} else {
		labels := map[string]string{}
		var lastPtr *bgwriterStats
		if c.lastBgwriterStats != nil && c.lastTime > 0 && deltaT > 0 {
			lastPtr = c.lastBgwriterStats
		}

		for _, m := range bgwriterMetrics {
			val := m.getVal(b, lastPtr, deltaT)
			// Don't report 0 rates on first run
			if m.unit == "rate" && lastPtr == nil {
				continue
			}
			dps = append(dps, metrics.DataPoint{
				Name:      m.name,
				Timestamp: timestamp,
				Value:     val,
				Labels:    labels,
			})
		}
		c.lastBgwriterStats = b
	}

	c.lastTime = timestamp

	return dps, nil
}

func (c *PostgresqlCollector) Discover() ([]collection.Metric, error) {
	var discovered []collection.Metric

	for _, m := range dbMetrics {
		discovered = append(discovered, collection.Metric{
			Name:   m.name,
			Type:   "gauge",
			Unit:   m.unit,
			Labels: map[string]string{},
		})
	}
	for _, m := range bgwriterMetrics {
		discovered = append(discovered, collection.Metric{
			Name:   m.name,
			Type:   "gauge",
			Unit:   m.unit,
			Labels: map[string]string{},
		})
	}

	return discovered, nil
}
