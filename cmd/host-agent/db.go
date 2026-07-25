// SPDX-License-Identifier: GPL-3.0-or-later
//
// db.go — optional PostgreSQL health metrics for huginn.
//
// Activated when the YAML config contains a non-empty `databases:` section.
// One connection is opened per DSN; metrics are collected on the push interval.
//
// Emitted metrics (all labelled host= and db=):
//   huginn_pg_up                  1 if reachable, 0 if not
//   huginn_pg_size_bytes          total on-disk size of the database
//   huginn_pg_connections         active backend connections
//   huginn_pg_commits_total       cumulative transaction commits
//   huginn_pg_rollbacks_total     cumulative transaction rollbacks
//   huginn_pg_cache_hit_ratio     blks_hit / (blks_hit + blks_read), 0-1
//   huginn_pg_deadlocks_total     cumulative deadlocks detected
//   huginn_pg_locks               current lock count

package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// DBConfig is one monitored database entry in huginn.yaml.
type DBConfig struct {
	Name string `yaml:"name"` // logical label used in metrics
	DSN  string `yaml:"dsn"`  // postgres://user:pass@host:port/dbname?sslmode=disable
}

// DBSnapshot holds one scrape of pg_stat_database for a single DB.
type DBSnapshot struct {
	Name        string
	Up          float64
	SizeBytes   float64
	Connections float64
	Commits     float64
	Rollbacks   float64
	CacheHit    float64
	Deadlocks   float64
	Locks       float64
}

// dbCollector manages open connections and periodic scrapes.
type dbCollector struct {
	cfgs []DBConfig
	dbs  []*sql.DB
}

func newDBCollector(cfgs []DBConfig) *dbCollector {
	c := &dbCollector{cfgs: cfgs}
	for _, cfg := range cfgs {
		db, err := sql.Open("postgres", cfg.DSN)
		if err != nil {
			fmt.Printf("huginn: db open %q: %v\n", cfg.Name, err)
			c.dbs = append(c.dbs, nil)
			continue
		}
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		db.SetConnMaxLifetime(5 * time.Minute)
		c.dbs = append(c.dbs, db)
	}
	return c
}

func (c *dbCollector) collect() []DBSnapshot {
	snaps := make([]DBSnapshot, len(c.cfgs))
	for i, cfg := range c.cfgs {
		snaps[i].Name = cfg.Name
		db := c.dbs[i]
		if db == nil {
			continue
		}

		// Ping first — drives huginn_pg_up
		if err := db.Ping(); err != nil {
			snaps[i].Up = 0
			continue
		}
		snaps[i].Up = 1

		// pg_stat_database row for the connected database
		row := db.QueryRow(`
			SELECT
				pg_database_size(current_database()),
				numbackends,
				xact_commit,
				xact_rollback,
				CASE WHEN blks_hit + blks_read = 0 THEN 0
				     ELSE blks_hit::float / (blks_hit + blks_read)
				END,
				deadlocks
			FROM pg_stat_database
			WHERE datname = current_database()`)

		var size, conns, commits, rollbacks, cacheHit, deadlocks float64
		if err := row.Scan(&size, &conns, &commits, &rollbacks, &cacheHit, &deadlocks); err == nil {
			snaps[i].SizeBytes = size
			snaps[i].Connections = conns
			snaps[i].Commits = commits
			snaps[i].Rollbacks = rollbacks
			snaps[i].CacheHit = cacheHit
			snaps[i].Deadlocks = deadlocks
		}

		// Total lock count across all relations
		lockRow := db.QueryRow(`SELECT count(*) FROM pg_locks`)
		var locks float64
		if err := lockRow.Scan(&locks); err == nil {
			snaps[i].Locks = locks
		}
	}
	return snaps
}

// promTextDB renders DB snapshots as Prometheus exposition lines.
func promTextDB(host string, snaps []DBSnapshot) string {
	if len(snaps) == 0 {
		return ""
	}
	var b strings.Builder
	for _, s := range snaps {
		lbl := fmt.Sprintf("{host=%q,db=%q}", host, s.Name)
		fmt.Fprintf(&b, "huginn_pg_up%s %g\n", lbl, s.Up)
		if s.Up == 0 {
			continue
		}
		fmt.Fprintf(&b, "huginn_pg_size_bytes%s %g\n", lbl, s.SizeBytes)
		fmt.Fprintf(&b, "huginn_pg_connections%s %g\n", lbl, s.Connections)
		fmt.Fprintf(&b, "huginn_pg_commits_total%s %g\n", lbl, s.Commits)
		fmt.Fprintf(&b, "huginn_pg_rollbacks_total%s %g\n", lbl, s.Rollbacks)
		fmt.Fprintf(&b, "huginn_pg_cache_hit_ratio%s %g\n", lbl, s.CacheHit)
		fmt.Fprintf(&b, "huginn_pg_deadlocks_total%s %g\n", lbl, s.Deadlocks)
		fmt.Fprintf(&b, "huginn_pg_locks%s %g\n", lbl, s.Locks)
	}
	return b.String()
}
