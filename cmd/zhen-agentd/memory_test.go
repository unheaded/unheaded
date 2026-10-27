// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/champion"
)

// S-CHAMPION-HARDENING Step 209 (ADR-059 Phase 2b): zhen-cli's /recall and
// /remember reach The Well through zhen-agentd, never a driver of their own.
// Recall is a read; remember is the mutating memory_write tool and passes
// the Champion gate.

type fakeMemStore struct {
	mu      sync.Mutex
	rows    []champion.MemoryRecord
	queries []string
}

func (f *fakeMemStore) Remember(_ context.Context, m champion.MemoryRecord) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m.ID = int64(100 + len(f.rows))
	f.rows = append(f.rows, m)
	return m.ID, nil
}

// Recall returns the rows newest-first with a made-up score: ranking is the
// SQL store's job; the handler must keep the order and honour k.
func (f *fakeMemStore) Recall(_ context.Context, q string, k int) ([]champion.MemoryRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, q)
	var out []champion.MemoryRecord
	for i := len(f.rows) - 1; i >= 0 && len(out) < k; i-- {
		r := f.rows[i]
		r.Score = float64(len(out)+1) / 10
		out = append(out, r)
	}
	return out, nil
}

func memServer(t *testing.T, mem champion.MemoryStore) *server {
	t.Helper()
	root := t.TempDir()
	pool := newChampionPool(newMetricsActionStore(nopActionStore{}), nil)
	pool.memory = mem
	return &server{pool: pool, defaultRoot: root, allowed: map[string]struct{}{root: {}}, memory: mem}
}

func TestMemoryRecall_ReturnsRowsWithProvenance(t *testing.T) {
	mem := &fakeMemStore{rows: []champion.MemoryRecord{
		{ID: 1, Question: "old", Answer: "a1", Source: "user", Model: "qwen-7b", CreatedAt: time.Unix(1, 0).UTC()},
		{ID: 2, Question: "new", Answer: "a2", Source: "ui:stevie", CreatedAt: time.Unix(2, 0).UTC()},
	}}
	srv := memServer(t, mem)
	rec := as("stevie", srv.handleMemoryRecall, http.MethodGet, "/api/v1/memory/recall?q=list+runbooks&k=1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("recall: %d %s", rec.Code, rec.Body.String())
	}
	var resp memoryRecallResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Memories) != 1 || resp.Memories[0].ID != 2 || resp.Memories[0].Source != "ui:stevie" {
		t.Fatalf("memories %+v, want only #2 with its source", resp.Memories)
	}
	if mem.queries[0] != "list runbooks" {
		t.Errorf("query %q", mem.queries[0])
	}
}

func TestMemoryRecall_Refusals(t *testing.T) {
	srv := memServer(t, &fakeMemStore{})
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/api/v1/memory/recall", http.StatusBadRequest},
		{http.MethodGet, "/api/v1/memory/recall?q=" + strings.Repeat("x", maxRecallQueryBytes+1), http.StatusBadRequest},
		{http.MethodGet, "/api/v1/memory/recall?q=x&k=0", http.StatusBadRequest},
		{http.MethodGet, "/api/v1/memory/recall?q=x&k=abc", http.StatusBadRequest},
		{http.MethodPost, "/api/v1/memory/recall?q=x", http.StatusMethodNotAllowed},
	} {
		if rec := as("stevie", srv.handleMemoryRecall, tc.method, tc.path, nil); rec.Code != tc.want {
			t.Errorf("%s %s: %d, want %d", tc.method, tc.path, rec.Code, tc.want)
		}
	}
	none := memServer(t, nil)
	if rec := as("stevie", none.handleMemoryRecall, http.MethodGet, "/api/v1/memory/recall?q=x", nil); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("no store: %d, want 503", rec.Code)
	}
}

func TestMemoryRemember_DirectUserStoresThroughGate(t *testing.T) {
	mem := &fakeMemStore{}
	srv := memServer(t, mem)
	rec := as("stevie", srv.handleMemoryRemember, http.MethodPost, "/api/v1/memory/remember",
		map[string]any{"question": "how do I list runbooks?", "answer": "/runbook list", "model": "qwen-7b", "emitted_by": "zhen-cli"})
	if rec.Code != http.StatusOK {
		t.Fatalf("remember: %d %s", rec.Code, rec.Body.String())
	}
	var resp toolExecResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	res, _ := resp.Result.(map[string]any)
	if resp.Status != "ok" || res["id"] != float64(100) {
		t.Fatalf("response %+v, want ok with id 100", resp)
	}
	if len(mem.rows) != 1 || mem.rows[0].Source != "ui:stevie" {
		t.Fatalf("rows %+v, want one from ui:stevie", mem.rows)
	}
}

func TestMemoryRemember_Refusals(t *testing.T) {
	mem := &fakeMemStore{}
	srv := memServer(t, mem)
	// No authenticated principal: nothing it could confirm, so refused.
	rec := as("", srv.handleMemoryRemember, http.MethodPost, "/api/v1/memory/remember",
		map[string]any{"question": "q", "answer": "a"})
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "authenticated") {
		t.Errorf("anonymous: %d %s, want 403", rec.Code, rec.Body.String())
	}
	// Rule 3 on the remembered text.
	rec = as("stevie", srv.handleMemoryRemember, http.MethodPost, "/api/v1/memory/remember",
		map[string]any{"question": "cleanup", "answer": "just rm -rf / it"})
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "destructive") {
		t.Errorf("destructive: %d %s, want 403", rec.Code, rec.Body.String())
	}
	// Missing answer.
	rec = as("stevie", srv.handleMemoryRemember, http.MethodPost, "/api/v1/memory/remember", map[string]any{"question": "q"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("no answer: %d, want 400", rec.Code)
	}
	if len(mem.rows) != 0 {
		t.Fatalf("stored %+v", mem.rows)
	}
	none := memServer(t, nil)
	rec = as("stevie", none.handleMemoryRemember, http.MethodPost, "/api/v1/memory/remember", map[string]any{"question": "q", "answer": "a"})
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("no store: %d, want 503", rec.Code)
	}
}

// The SQL store against a real unheaded_app (skipped without one).
func TestPGMemoryStore_Live(t *testing.T) {
	dsn := os.Getenv("WELL_TEST_APP_DSN")
	if dsn == "" {
		t.Skip("WELL_TEST_APP_DSN unset")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	st, err := newPGMemoryStore(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	marker := "zhen-agentd-test-" + time.Now().Format("150405.000000")
	id, err := st.Remember(ctx, champion.MemoryRecord{Question: marker + " list runbooks", Answer: "use /runbook list", Source: "ui:test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, `DELETE FROM zhen_memories WHERE id = $1`, id) })
	rows, err := st.Recall(ctx, marker, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 || rows[0].ID != id || rows[0].Source != "ui:test" {
		t.Fatalf("recall %+v, want row %d first", rows, id)
	}
}
