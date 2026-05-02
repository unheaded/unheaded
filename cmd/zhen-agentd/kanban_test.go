// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

// Hop-4 integration: user → zhen-agentd → Champion → KanbanStore.
// Closes the loop opened in commit 1b1777c2 where -action-store=pg
// shipped but -kanban-store wasn't a flag at all, and any agent-emitted
// kanban_create / kanban_update tool call passed the gate then failed
// at dispatch with "no kanban store configured".

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"unheaded/pkg/agent"
	"unheaded/pkg/champion"
)

// --- in-memory KanbanStore for tests that don't need real PG ---

// memKanbanStore satisfies pkg/champion.KanbanStore with a sync.Mutex-
// guarded map. Sufficient for verifying the dispatch path is wired and
// the Champion can hand off to a real store.
type memKanbanStore struct {
	mu    sync.Mutex
	tasks map[string]champion.KanbanTask
}

func newMemKanbanStore() *memKanbanStore {
	return &memKanbanStore{tasks: make(map[string]champion.KanbanTask)}
}

func (m *memKanbanStore) CreateTask(_ context.Context, t *champion.KanbanTask) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.tasks[t.ID]; exists {
		return fmt.Errorf("task %q already exists", t.ID)
	}
	m.tasks[t.ID] = *t
	return nil
}

func (m *memKanbanStore) UpdateTask(_ context.Context, id string, updates map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return fmt.Errorf("task %q not found", id)
	}
	if v, ok := updates["title"].(string); ok {
		t.Title = v
	}
	if v, ok := updates["status"].(string); ok {
		t.Status = v
	}
	m.tasks[id] = t
	return nil
}

func (m *memKanbanStore) GetTask(_ context.Context, id string) (*champion.KanbanTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return nil, nil
	}
	return &t, nil
}

func (m *memKanbanStore) ListTasks(_ context.Context) ([]champion.KanbanTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]champion.KanbanTask, 0, len(m.tasks))
	for _, t := range m.tasks {
		out = append(out, t)
	}
	return out, nil
}

// newKanbanDaemon builds a daemon wired to the supplied KanbanStore.
func newKanbanDaemon(t *testing.T, kStore champion.KanbanStore, retr agent.Retriever, llm agent.LLM) (*httptest.Server, string) {
	t.Helper()
	root := t.TempDir()
	store := newMetricsActionStore(nopActionStore{})
	pool := newChampionPool(store, kStore)
	srv := &server{
		pool:        pool,
		defaultRoot: root,
		allowed:     map[string]struct{}{root: {}},
		retriever:   retr,
		llm:         llm,
		vorURL:      "http://127.0.0.1:1",
		llamaURL:    "http://127.0.0.1:1",
		ready:       newReadyTracker(&http.Client{}, "http://127.0.0.1:1", "http://127.0.0.1:1"),
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/api/v1/agent/ask", instrument("/api/v1/agent/ask", http.HandlerFunc(srv.handleAsk)))
	mux.Handle("/api/v1/agent/confirm", instrument("/api/v1/agent/confirm", http.HandlerFunc(srv.handleConfirm)))
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, root
}

// --- happy path with in-memory store ---

// TestKanban_AskCreate_CanonicalJustification verifies that with
// trusted (canonical) justification, kanban_create flows through the
// gate and lands in the store WITHOUT a confirm round-trip — the
// happy path that we couldn't even test before this commit because
// the dispatch always failed with "no kanban store configured".
func TestKanban_AskCreate_CanonicalJustification(t *testing.T) {
	mem := newMemKanbanStore()
	llm := &scriptedLLM{out: []string{
		`{"thought":"need a task","tool_call":{"name":"kanban_create","args":{"task":{"id":"task-001","title":"close hop 4","status":"todo"}}}}`,
		`{"thought":"observed","answer":"task created"}`,
	}}
	ts, _ := newKanbanDaemon(t, mem, canonicalStub(), llm)

	resp := mustPostJSON(t, ts.URL+"/api/v1/agent/ask", map[string]any{
		"goal": "create the task",
	})
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	var ar askResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Find the kanban_create turn — it must have run (no Refused, no
	// Pending) and produced a clean observation.
	var tcTurn *traceEntry
	for i := range ar.Trace {
		if ar.Trace[i].Tool == "kanban_create" {
			tcTurn = &ar.Trace[i]
			break
		}
	}
	if tcTurn == nil {
		t.Fatalf("no kanban_create turn in trace; got %+v", ar.Trace)
	}
	if tcTurn.Refused {
		t.Fatalf("kanban_create was refused with canonical justification: obs=%q", tcTurn.Observation)
	}
	if strings.Contains(tcTurn.Observation, "no kanban store configured") {
		t.Fatalf("dispatch still hit the legacy 'no kanban store' branch: %q", tcTurn.Observation)
	}

	// Verify side effect: the task landed in the store.
	got, err := mem.GetTask(context.Background(), "task-001")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got == nil {
		t.Fatalf("task task-001 not in store; tasks=%+v", mem.tasks)
	}
	if got.Title != "close hop 4" {
		t.Errorf("title: got %q, want %q", got.Title, "close hop 4")
	}
	if got.Status != "todo" {
		t.Errorf("status: got %q, want todo", got.Status)
	}
}

// TestKanban_AskCreate_UntrustedRequiresConfirm verifies the same call
// with external (untrusted) justification triggers Rule 2 → pending
// token → /confirm → store gets the task. The full hop-4 chain under
// the security model.
func TestKanban_AskCreate_UntrustedRequiresConfirm(t *testing.T) {
	mem := newMemKanbanStore()
	llm := &scriptedLLM{out: []string{
		`{"thought":"create from external doc","tool_call":{"name":"kanban_create","args":{"task":{"id":"task-002","title":"from external","status":"todo"}}}}`,
		`{"thought":"observed refusal","answer":"awaiting confirm"}`,
	}}
	ts, root := newKanbanDaemon(t, mem, externalStub(), llm)

	// 1. /ask → pending token.
	resp := mustPostJSON(t, ts.URL+"/api/v1/agent/ask", map[string]any{
		"goal": "create from external corpus",
	})
	defer resp.Body.Close()
	var ar askResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var token string
	for _, e := range ar.Trace {
		if e.PendingToken != "" {
			token = e.PendingToken
			break
		}
	}
	if token == "" {
		t.Fatalf("no pending_token in trace; got %+v", ar.Trace)
	}

	// 2. Pre-confirm: store must be empty.
	if got, _ := mem.GetTask(context.Background(), "task-002"); got != nil {
		t.Fatalf("pre-confirm: task task-002 unexpectedly in store: %+v", got)
	}

	// 3. /confirm.
	confirmResp := mustPostJSON(t, ts.URL+"/api/v1/agent/confirm", map[string]any{
		"token":        token,
		"project_root": root,
	})
	defer confirmResp.Body.Close()
	if confirmResp.StatusCode != 200 {
		t.Fatalf("confirm status: got %d, want 200", confirmResp.StatusCode)
	}
	var cr confirmResponse
	if err := json.NewDecoder(confirmResp.Body).Decode(&cr); err != nil {
		t.Fatalf("decode confirm: %v", err)
	}
	if cr.Status != "ok" {
		t.Fatalf("confirm status: got %q, want ok (reason=%q)", cr.Status, cr.Reason)
	}

	// 4. Post-confirm: task is in the store.
	got, err := mem.GetTask(context.Background(), "task-002")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got == nil {
		t.Fatalf("post-confirm: task task-002 NOT in store; tasks=%+v", mem.tasks)
	}
	if got.Title != "from external" {
		t.Errorf("title: got %q, want %q", got.Title, "from external")
	}
}

// TestKanban_NoStore_DispatchStillFailsCleanly verifies the legacy path
// still produces a clean error when kanban store is nil (the default
// "memory" mode in main.go means no store, intentionally — the daemon
// shouldn't silently invent one). Champion's dispatch should surface
// "no kanban store configured" in the observation rather than panic.
func TestKanban_NoStore_DispatchSurfacesError(t *testing.T) {
	llm := &scriptedLLM{out: []string{
		`{"thought":"create","tool_call":{"name":"kanban_create","args":{"task":{"id":"task-003","title":"x","status":"todo"}}}}`,
		`{"thought":"observed","answer":"failed"}`,
	}}
	ts, _ := newKanbanDaemon(t, nil, canonicalStub(), llm)

	resp := mustPostJSON(t, ts.URL+"/api/v1/agent/ask", map[string]any{"goal": "create"})
	defer resp.Body.Close()
	var ar askResponse
	json.NewDecoder(resp.Body).Decode(&ar)

	var tcTurn *traceEntry
	for i := range ar.Trace {
		if ar.Trace[i].Tool == "kanban_create" {
			tcTurn = &ar.Trace[i]
			break
		}
	}
	if tcTurn == nil {
		t.Fatalf("no kanban_create turn: %+v", ar.Trace)
	}
	if !strings.Contains(strings.ToLower(tcTurn.Observation), "kanban store") {
		t.Errorf("observation should mention 'kanban store'; got %q", tcTurn.Observation)
	}
}

// --- live PG variant, gated on WELL_DSN ---

// TestKanban_PG_LiveSmoke is opt-in. Set WELL_DSN to a reachable
// Postgres (with a sane sslmode) to exercise the full pgKanbanStore →
// SaveTask → row read-back path. Skipped in CI by default; run via:
//
//	WELL_DSN='host=127.0.0.1 user=unheaded password=... dbname=the_well sslmode=disable' \
//	  go test ./cmd/zhen-agentd/ -run Kanban_PG_LiveSmoke -v
func TestKanban_PG_LiveSmoke(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("WELL_DSN"))
	if dsn == "" {
		t.Skip("WELL_DSN not set; skipping live PG kanban smoke")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		t.Skipf("WELL_DSN unreachable: %v", err)
	}
	ks, err := newPGKanbanStore(pingCtx, db)
	if err != nil {
		t.Fatalf("newPGKanbanStore: %v", err)
	}

	// Use a unique task ID so the test is idempotent across runs.
	taskID := fmt.Sprintf("zhen-agentd-kanban-test-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM kanban_tasks WHERE id=$1`, taskID)
	})

	llm := &scriptedLLM{out: []string{
		fmt.Sprintf(`{"thought":"create","tool_call":{"name":"kanban_create","args":{"task":{"id":%q,"title":"hop4-smoke","status":"todo"}}}}`, taskID),
		`{"thought":"end","answer":"done"}`,
	}}
	ts, _ := newKanbanDaemon(t, ks, canonicalStub(), llm)

	resp := mustPostJSON(t, ts.URL+"/api/v1/agent/ask", map[string]any{"goal": "live smoke"})
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("ask status: got %d, want 200", resp.StatusCode)
	}

	// Read it back through the adapter.
	got, err := ks.GetTask(context.Background(), taskID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatalf("task %s not in PG", taskID)
	}
	if got.Title != "hop4-smoke" {
		t.Errorf("title: got %q, want hop4-smoke", got.Title)
	}
}
