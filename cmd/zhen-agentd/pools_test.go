// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The board's DSN alone does not open Zhen's tables: they are in
// unheaded_zhen since ADR-149, and every store that needs them says which
// variable is missing.
func TestOpenPools_NeedsZhenDSN(t *testing.T) {
	t.Setenv("WELL_ZHEN_DSN", "")
	t.Setenv("WELL_APP_ZHEN_DSN", "postgres://app_zhen:x@127.0.0.1:1/unheaded_app?sslmode=disable&connect_timeout=2")
	ctx := context.Background()
	if _, _, err := buildMemoryStore(ctx, "pg"); err == nil || !strings.Contains(err.Error(), "WELL_ZHEN_DSN") {
		t.Errorf("-memory-store pg: %v", err)
	}
	if _, err := buildActionStore("pg"); err == nil || !strings.Contains(err.Error(), "WELL_ZHEN_DSN") {
		t.Errorf("-action-store=pg: %v", err)
	}
	if _, _, err := openWorkerStore(ctx); err == nil || !strings.Contains(err.Error(), "WELL_ZHEN_DSN") {
		t.Errorf("-worker: %v", err)
	}
}

// The worker needs the board too, and says so.
func TestOpenPools_WorkerNeedsBoardDSN(t *testing.T) {
	p := &wellPools{getenv: func(string) string { return "" }}
	if _, err := p.boardPool(context.Background(), "-worker"); err == nil || !strings.Contains(err.Error(), "WELL_APP_ZHEN_DSN") {
		t.Errorf("board: %v", err)
	}
}

// Every app_zhen client that can run at once stays under the role's
// connection limit in migration 051 (a role's limit counts its connections
// in every database). REV-5 (independent review 2026-11-15): the old test
// counted one zhen-agentd and zhen-claude's 2 and missed the overnight
// process, zhen-claude review (two pools) and the web app.
func TestPoolCaps(t *testing.T) {
	daemon := (&wellPools{}).caps()
	if daemon != [2]int{6, 2} {
		t.Fatalf("daemon caps %v, want [6 2]", daemon)
	}
	night := (&wellPools{overnight: true}).caps()
	if night != [2]int{2, 1} {
		t.Fatalf("overnight caps %v, want [2 1]: one card at a time and its lease", night)
	}
	const (
		claudeReview = 2 + 2 // zhen-claude review: runs and cards, openWellDB caps each at 2
		claudeQueue  = 2     // a second zhen-claude: confirm, or a session with -confirm queue
		webApp       = 2     // raft/zhen_app.py: Zhen's rows and the board's reads (REV-2)
		operator     = 1     // one psql as app_zhen
	)
	need := daemon[0] + daemon[1] + night[0] + night[1] + claudeReview + claudeQueue + webApp + operator
	b, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "051_zhen_database.sql"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^ALTER ROLE app_zhen CONNECTION LIMIT (\d+);`).FindSubmatch(b)
	if m == nil {
		t.Fatal("051 sets no CONNECTION LIMIT for app_zhen")
	}
	limit, _ := strconv.Atoi(string(m[1]))
	if need > limit {
		t.Fatalf("app_zhen clients can open %d connections; 051's limit is %d", need, limit)
	}
	y, err := os.ReadFile(filepath.Join("..", "..", "modules", "zhen", "module.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(y), "connection_limit: "+strconv.Itoa(limit)+"}") {
		t.Fatalf("modules/zhen/module.yaml does not restate the limit %d", limit)
	}
}
