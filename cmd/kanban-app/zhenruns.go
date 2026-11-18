// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"database/sql"
	"os"
	"strings"

	"unheaded/pkg/database"
	"unheaded/pkg/modules"
)

// zhenRunsDB is the default for WELL_ZHEN_DB: Zhen's database (ADR-149).
const zhenRunsDB = "unheaded_zhen"

// zhenRunsMaxConns caps the runs pool: one card detail read at a time is
// the load, and app_kanban's connections are the board's.
const zhenRunsMaxConns = 2

// zhenRuns is a pool on Zhen's database with the board's own login and
// settings (database.AppKanbanConfig, DBName from WELL_ZHEN_DB), opened on
// first use while the Zhen module is on and kept. nil when the module is
// off, the board is not the Well, or the open fails (one warning per process,
// agentRunWarned; the next read tries again).
func (s *Server) zhenRuns(ctx context.Context) *sql.DB {
	if !modules.Enabled("zhen") {
		return nil
	}
	if _, onWell := s.store.(*PgStore); !onWell {
		return nil
	}
	s.runsMu.Lock()
	defer s.runsMu.Unlock()
	if s.runsDB != nil {
		return s.runsDB
	}
	cfg := database.AppKanbanConfig()
	cfg.DBName = zhenRunsDB
	if v := strings.TrimSpace(os.Getenv("WELL_ZHEN_DB")); v != "" {
		cfg.DBName = v
	}
	open := s.openRuns
	if open == nil {
		open = func(ctx context.Context, c database.Config) (*sql.DB, error) {
			db, err := database.Connect(ctx, c)
			if err != nil {
				return nil, err
			}
			db.SetMaxOpenConns(zhenRunsMaxConns)
			db.SetMaxIdleConns(zhenRunsMaxConns)
			return db, nil
		}
	}
	db, err := open(ctx, cfg)
	if err != nil {
		if agentRunWarned.CompareAndSwap(false, true) {
			log.Warn().Err(err).Str("db", cfg.DBName).Msg("the Zhen module is on but its runs are not readable: cards show no agent run")
		}
		return nil
	}
	s.runsDB = db
	return db
}
