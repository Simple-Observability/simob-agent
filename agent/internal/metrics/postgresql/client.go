package postgresql

import (
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type dbStats struct {
	Active       float64
	XactCommit   float64
	XactRollback float64
	BlksRead     float64
	BlksHit      float64
	TupReturned  float64
	TupFetched   float64
	TupInserted  float64
	TupUpdated   float64
	TupDeleted   float64
	Conflicts    float64
	Deadlocks    float64
}

type bgwriterStats struct {
	CheckpointsTimed  float64
	CheckpointsReq    float64
	BuffersCheckpoint float64
	BuffersClean      float64
	BuffersBackend    float64
	BuffersAlloc      float64
}

type databaseStat struct {
	Name  string
	Stats dbStats
}

type postgreSQLClient interface {
	getDatabaseStats() ([]databaseStat, error)
	getBgwriterStats() (*bgwriterStats, error)
	close() error
}

type realPostgreSQLClient struct {
	db *sql.DB
}

func (c *realPostgreSQLClient) getDatabaseStats() ([]databaseStat, error) {
	rows, err := c.db.Query(`
		SELECT
			datname, numbackends, xact_commit, xact_rollback, blks_read, blks_hit,
			tup_returned, tup_fetched, tup_inserted, tup_updated, tup_deleted,
			conflicts, deadlocks
		FROM pg_stat_database
		WHERE datname IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []databaseStat
	for rows.Next() {
		var s databaseStat
		err := rows.Scan(
			&s.Name, &s.Stats.Active, &s.Stats.XactCommit, &s.Stats.XactRollback, &s.Stats.BlksRead, &s.Stats.BlksHit,
			&s.Stats.TupReturned, &s.Stats.TupFetched, &s.Stats.TupInserted, &s.Stats.TupUpdated, &s.Stats.TupDeleted,
			&s.Stats.Conflicts, &s.Stats.Deadlocks)
		if err != nil {
			continue
		}
		stats = append(stats, s)
	}
	return stats, nil
}

func (c *realPostgreSQLClient) getBgwriterStats() (*bgwriterStats, error) {
	row := c.db.QueryRow(`
		SELECT
			checkpoints_timed, checkpoints_req, buffers_checkpoint,
			buffers_clean, buffers_backend, buffers_alloc
		FROM pg_stat_bgwriter`)
	var b bgwriterStats
	err := row.Scan(
		&b.CheckpointsTimed, &b.CheckpointsReq, &b.BuffersCheckpoint,
		&b.BuffersClean, &b.BuffersBackend, &b.BuffersAlloc)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (c *realPostgreSQLClient) close() error {
	return c.db.Close()
}
