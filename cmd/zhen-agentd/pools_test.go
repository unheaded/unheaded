// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
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

// Two app_zhen pools whose caps, with zhen-claude's 2, stay under the
// role's connection limit of 12 (migration 051, D6).
func TestPoolCaps(t *testing.T) {
	if zhenPoolMax != 6 || boardPoolMax != 2 {
		t.Fatalf("caps %d + %d, want 6 + 2", zhenPoolMax, boardPoolMax)
	}
	if zhenPoolMax+boardPoolMax+2 > 12 {
		t.Fatal("the caps exceed app_zhen's connection limit")
	}
}
