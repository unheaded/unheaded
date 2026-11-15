// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// zhen-agentd talks to the Well as app_zhen through two pools, opened once
// and shared (ADR-149; S-ZHEN-SWITCH Step 209, Erda F6: it used to open
// seven pools, 21 connections):
//
//   - the Zhen pool, WELL_ZHEN_DSN: unheaded_zhen, Zhen's own tables
//     (zhen_actions, zhen_memories, zhen_pending_confirmations,
//     zhen_agent_runs); the action store, memory store, confirm queue,
//     scheduler and worker share it;
//   - the board pool, WELL_APP_ZHEN_DSN: unheaded_app, the board only
//     (kanban_tasks, kanban_zhen_optin); only the worker needs it.
//
// app_kanban (WELL_KANBAN_DSN, the scheduler's cards) and ops_writer
// (WELL_OPS_DSN, the audit) keep their own pools: other roles.
const (
	zhenPoolMax  = 6
	boardPoolMax = 2
)

// wellPools opens each pool on first use and keeps it; a failed open is not
// kept, so the next caller tries again.
type wellPools struct {
	mu     sync.Mutex
	getenv func(string) string
	zhenDB *sql.DB
	board  *sql.DB
}

var pools = &wellPools{getenv: os.Getenv}

// zhen is the Zhen pool; what names the flag that needs it, for the error.
func (p *wellPools) zhen(ctx context.Context, what string) (*sql.DB, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.zhenDB != nil {
		return p.zhenDB, nil
	}
	db, err := p.open(ctx, "WELL_ZHEN_DSN", "unheaded_zhen", what, zhenPoolMax)
	if err != nil {
		return nil, err
	}
	p.zhenDB = db
	return db, nil
}

// boardPool is the board pool.
func (p *wellPools) boardPool(ctx context.Context, what string) (*sql.DB, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.board != nil {
		return p.board, nil
	}
	db, err := p.open(ctx, "WELL_APP_ZHEN_DSN", "unheaded_app", what, boardPoolMax)
	if err != nil {
		return nil, err
	}
	p.board = db
	return db, nil
}

func (p *wellPools) open(ctx context.Context, env, dbname, what string, max int) (*sql.DB, error) {
	dsn := strings.TrimSpace(p.getenv(env))
	if dsn == "" {
		return nil, fmt.Errorf("%s needs %s (%s as app_zhen)", what, env, dbname)
	}
	warnIfInsecureDSN(dsn)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("%s: %s: %w", what, env, err)
	}
	db.SetMaxOpenConns(max)
	db.SetMaxIdleConns(max)
	db.SetConnMaxLifetime(5 * time.Minute)
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("%s: the Well is unreachable (%s): %w", what, env, err)
	}
	return db, nil
}

// close closes whatever was opened; main defers it.
func (p *wellPools) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.zhenDB != nil {
		_ = p.zhenDB.Close()
		p.zhenDB = nil
	}
	if p.board != nil {
		_ = p.board.Close()
		p.board = nil
	}
}
