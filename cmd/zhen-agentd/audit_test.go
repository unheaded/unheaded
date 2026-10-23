// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"unheaded/pkg/agent"
	"unheaded/pkg/champion"
	"unheaded/pkg/claudebridge"
)

type recAudit struct {
	mu    sync.Mutex
	reqs  []claudebridge.RequestRecord
	tools []claudebridge.ToolRecord
}

func (a *recAudit) Request(_ context.Context, r claudebridge.RequestRecord) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.reqs = append(a.reqs, r)
}

func (a *recAudit) ToolCall(_ context.Context, r claudebridge.ToolRecord) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tools = append(a.tools, r)
}

func fakeLlama(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLlamaLLM_AuditsOneLocalRowPerCall(t *testing.T) {
	srv := fakeLlama(t, 200, `{"model":"qwen-7b.gguf","choices":[{"finish_reason":"stop","message":{"content":"the answer S2B-SECRET-MARKER"}}],"usage":{"prompt_tokens":120,"completion_tokens":30}}`)
	rec := &recAudit{}
	l := &llamaLLM{client: srv.Client(), baseURL: srv.URL, model: "qwen-7b", audit: rec}
	ctx := withRunID(context.Background())
	for i := 0; i < 2; i++ {
		if _, err := l.Complete(ctx, []agent.Message{{Role: "user", Content: "q S2B-SECRET-MARKER"}}, 64, 0, 42); err != nil {
			t.Fatal(err)
		}
	}
	if len(rec.reqs) != 2 {
		t.Fatalf("records = %d, want 2", len(rec.reqs))
	}
	r := rec.reqs[0]
	if r.Backend != claudebridge.BackendLocal || r.Cost != 0 || r.InputTokens != 120 || r.OutputTokens != 30 ||
		r.Model != "qwen-7b" || r.ServedBy != "qwen-7b.gguf" || r.StopReason != "stop" || !r.Sent || r.Err != "" {
		t.Errorf("record = %+v", r)
	}
	if rec.reqs[0].Conversation != rec.reqs[1].Conversation || rec.reqs[0].Conversation != runIDFrom(ctx) {
		t.Error("calls of one run do not share its conversation id")
	}
}

func TestLlamaLLM_AuditsFailures(t *testing.T) {
	srv := fakeLlama(t, 503, `loading model`)
	rec := &recAudit{}
	l := &llamaLLM{client: srv.Client(), baseURL: srv.URL, model: "qwen-7b", audit: rec}
	if _, err := l.Complete(context.Background(), []agent.Message{{Role: "user", Content: "q"}}, 64, 0, 0); err == nil {
		t.Fatal("503 accepted")
	}
	if len(rec.reqs) != 1 || rec.reqs[0].Err == "" || rec.reqs[0].Conversation == "" {
		t.Errorf("records = %+v", rec.reqs)
	}
}

func TestLlamaLLM_NoAuditorStillWorks(t *testing.T) {
	srv := fakeLlama(t, 200, `{"choices":[{"message":{"content":"ok"}}]}`)
	l := &llamaLLM{client: srv.Client(), baseURL: srv.URL, model: "m"}
	if got, err := l.Complete(context.Background(), []agent.Message{{Role: "user", Content: "q"}}, 8, 0, 0); err != nil || got != "ok" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestAuditActionStore_OneEventPerGateDecision(t *testing.T) {
	rec := &recAudit{}
	store := newAuditActionStore(&stderrActionStore{}, rec)
	root := t.TempDir()
	champ, err := champion.New(champion.Config{ProjectRoot: root, SessionID: "run-1"}, store)
	if err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(root, "a.txt")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := champ.Dispatch(ctx, champion.ToolCall{Name: "read_file", Args: map[string]any{"path": f}, EmittedBy: "zhen-agent/qwen-7b"}); err != nil {
		t.Fatal(err)
	}
	_, _ = champ.Dispatch(ctx, champion.ToolCall{Name: "write_file", Args: map[string]any{"path": f, "content": "S2B-SECRET-MARKER"}, EmittedBy: "zhen-agent/qwen-7b"})
	_, _ = champ.Dispatch(ctx, champion.ToolCall{Name: "write_file", Args: map[string]any{"path": f, "content": "rm -rf /"}, EmittedBy: "zhen-agent/qwen-7b"})

	if len(rec.tools) != 3 {
		t.Fatalf("gate events = %d, want 3: %+v", len(rec.tools), rec.tools)
	}
	got := []string{}
	for _, r := range rec.tools {
		got = append(got, r.Name+":"+r.Decision)
		if strings.Contains(r.Reason+r.InputSHA256+r.Name, "S2B-SECRET-MARKER") {
			t.Error("argument text reached the audit record")
		}
		if r.Backend != claudebridge.BackendLocal || r.Model != "zhen-agent/qwen-7b" || r.Conversation != "run-1" {
			t.Errorf("record = %+v", r)
		}
	}
	if strings.Join(got, ",") != "read_file:allowed,write_file:refused,write_file:refused" {
		t.Errorf("decisions = %v", got)
	}
	if !rec.tools[1].Pending || rec.tools[2].Pending {
		t.Errorf("pending flags: untrusted write %v, destructive write %v", rec.tools[1].Pending, rec.tools[2].Pending)
	}
}

func TestNewAuditActionStore_NilAuditorIsPassThrough(t *testing.T) {
	inner := &stderrActionStore{}
	if got := newAuditActionStore(inner, nil); got != champion.ActionStore(inner) {
		t.Error("nil auditor should leave the store as is")
	}
}

func TestBuildAgentdAudit_FlagWithoutEnvFails(t *testing.T) {
	t.Setenv("WELL_OPS_DSN", "")
	t.Setenv("WOTAN_ADDR", "")
	if _, err := buildAgentdAudit(context.Background(), true, false); err == nil || !strings.Contains(err.Error(), "WELL_OPS_DSN") {
		t.Errorf("-audit-well without env: %v", err)
	}
	if _, err := buildAgentdAudit(context.Background(), false, true); err == nil || !strings.Contains(err.Error(), "WOTAN_ADDR") {
		t.Errorf("-audit-wotan without env: %v", err)
	}
	a, err := buildAgentdAudit(context.Background(), false, false)
	if err != nil || a.auditor != nil {
		t.Errorf("no flags: %v, auditor %v", err, a.auditor)
	}
}

func TestBuildAgentdAudit_WellDownWarnsAndContinues(t *testing.T) {
	t.Setenv("WELL_OPS_DSN", "postgres://ops_writer:x@127.0.0.1:1/unheaded_ops?sslmode=disable&connect_timeout=2")
	a, err := buildAgentdAudit(context.Background(), true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer a.close()
	if a.well != nil || a.auditor != nil {
		t.Error("an unreachable Well was wired as a sink")
	}
}
