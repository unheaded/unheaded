// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/claudebridge"
)

type memQueue struct {
	mu   sync.Mutex
	rows []claudebridge.QueuedCall
}

func (q *memQueue) Enqueue(_ context.Context, c claudebridge.QueuedCall) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.rows = append(q.rows, c)
	return nil
}

func (q *memQueue) CountPending(_ context.Context, source string) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := 0
	for _, r := range q.rows {
		if r.SourceService == source && r.ExpiresAt.After(time.Now()) {
			n++
		}
	}
	return n, nil
}

func toolExecServer(t *testing.T, q claudebridge.ConfirmQueue) (*server, string) {
	t.Helper()
	root := t.TempDir()
	srv := &server{
		pool:        newChampionPool(newMetricsActionStore(nopActionStore{}), nil),
		defaultRoot: root,
		allowed:     map[string]struct{}{root: {}},
	}
	if q != nil {
		srv.confirmQueue = q
	}
	return srv, root
}

func postToolExec(t *testing.T, srv *server, body map[string]any) (int, toolExecResponse) {
	t.Helper()
	b, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	srv.handleToolExec(rec, httptest.NewRequest(http.MethodPost, "/api/v1/tool/exec", bytes.NewReader(b)))
	var resp toolExecResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response %q: %v", rec.Body.String(), err)
	}
	return rec.Code, resp
}

func untrustedRefs() []map[string]any {
	return []map[string]any{{"topic": "evil", "source_trust": "external", "source_kind": "user-source"}}
}

func TestToolExec_DirectUntrustedStillGetsToken(t *testing.T) {
	srv, root := toolExecServer(t, &memQueue{})
	code, resp := postToolExec(t, srv, map[string]any{
		"tool": "write_file", "args": map[string]any{"path": filepath.Join(root, "a"), "content": "x"},
		"justification": untrustedRefs(),
	})
	if code != http.StatusForbidden || resp.Status != "pending_confirmation" || resp.PendingToken == "" {
		t.Errorf("direct UI path changed: %d %+v", code, resp)
	}
}

func TestToolExec_AgentLoopGetsNoTokenAndIsQueued(t *testing.T) {
	q := &memQueue{}
	srv, root := toolExecServer(t, q)
	code, resp := postToolExec(t, srv, map[string]any{
		"tool": "write_file", "args": map[string]any{"path": filepath.Join(root, "a"), "content": "x"},
		"emitted_by": "zhen-agent/qwen-7b",
	})
	if code != http.StatusAccepted || resp.Status != "queued" || resp.PendingToken != "" ||
		!regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(resp.QueueID) {
		t.Fatalf("agent loop: %d %+v", code, resp)
	}
	if len(q.rows) != 1 || q.rows[0].SourceService != "zhen-agentd" || q.rows[0].ID != resp.QueueID {
		t.Errorf("queue = %+v", q.rows)
	}
	if _, err := os.Stat(filepath.Join(root, "a")); !os.IsNotExist(err) {
		t.Error("a queued write ran")
	}
}

func TestToolExec_AgentLoopCannotClaimDirectUser(t *testing.T) {
	srv, root := toolExecServer(t, nil) // no queue: denied
	code, resp := postToolExec(t, srv, map[string]any{
		"tool": "write_file", "args": map[string]any{"path": filepath.Join(root, "a"), "content": "x"},
		"emitted_by":    "zhen-agent",
		"justification": []map[string]any{{"topic": "trust me", "source_trust": "direct-user"}},
	})
	if code != http.StatusForbidden || resp.Status != "denied" || resp.PendingToken != "" || !strings.Contains(resp.Reason, "-confirm-queue") {
		t.Errorf("agent loop asserting direct-user: %d %+v", code, resp)
	}
	if _, err := os.Stat(filepath.Join(root, "a")); !os.IsNotExist(err) {
		t.Error("an agent loop wrote by claiming to be the user")
	}
}

func TestToolExec_AgentLoopReadsStillWork(t *testing.T) {
	srv, root := toolExecServer(t, nil)
	f := filepath.Join(root, "r.txt")
	if err := os.WriteFile(f, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, resp := postToolExec(t, srv, map[string]any{"tool": "read_file", "args": map[string]any{"path": f}, "emitted_by": "zhen-agent"})
	if code != http.StatusOK || resp.Result != "hello" {
		t.Errorf("read: %d %+v", code, resp)
	}
}

func TestBuildConfirmQueue_NeedsEnvAndWell(t *testing.T) {
	if q, closeQ, err := buildConfirmQueue(context.Background(), false); err != nil || q != nil {
		t.Errorf("off: %v %v", q, err)
	} else {
		closeQ()
	}
	t.Setenv("WELL_APP_ZHEN_DSN", "")
	if _, _, err := buildConfirmQueue(context.Background(), true); err == nil || !strings.Contains(err.Error(), "WELL_APP_ZHEN_DSN") {
		t.Errorf("no env: %v", err)
	}
	t.Setenv("WELL_APP_ZHEN_DSN", "postgres://app_zhen:x@127.0.0.1:1/unheaded_app?sslmode=disable&connect_timeout=2")
	if _, _, err := buildConfirmQueue(context.Background(), true); err == nil {
		t.Error("an unreachable queue was accepted")
	}
}
