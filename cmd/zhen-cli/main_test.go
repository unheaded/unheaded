// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

// ADR-059 Phase 2/3 regression cover: every CLI mutation must go through
// cmd/zhen-agentd /api/v1/tool/exec — never bypass Champion's gate. These
// tests assert the wire shape and route, not Champion's gate behaviour
// (covered upstream by pkg/champion tests).
package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeAgentd captures requests to /api/v1/tool/exec so the test can
// assert the CLI sent the expected wire shape, and replies with the
// canonical response shape.
type fakeAgentd struct {
	mu       *struct{}
	srv      *httptest.Server
	captured []toolExecRequest
	respond  func(req toolExecRequest) toolExecResponse
}

func newFakeAgentd(t *testing.T, respond func(toolExecRequest) toolExecResponse) *fakeAgentd {
	t.Helper()
	f := &fakeAgentd{respond: respond}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/tool/exec", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var req toolExecRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		f.captured = append(f.captured, req)
		resp := f.respond(req)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func newTestSession(agentdURL string) *session {
	return &session{
		agentdURL: agentdURL,
		client:    &http.Client{Timeout: 5 * time.Second},
		maxChars:  200,
	}
}

// TestRunbookList_RoutesThroughToolExec asserts /runbook list emits a
// POST to /api/v1/tool/exec with tool="runbook_list" and emitted_by
// distinguishing the CLI from the web UI in audit trails.
func TestRunbookList_RoutesThroughToolExec(t *testing.T) {
	f := newFakeAgentd(t, func(req toolExecRequest) toolExecResponse {
		return toolExecResponse{Status: "ok", Result: []string{"observe/health-sweep"}}
	})
	s := newTestSession(f.srv.URL)
	s.handleRunbook([]string{"list"})

	if len(f.captured) != 1 {
		t.Fatalf("expected 1 captured request, got %d", len(f.captured))
	}
	got := f.captured[0]
	if got.Tool != "runbook_list" {
		t.Errorf("tool: got %q want %q", got.Tool, "runbook_list")
	}
	if got.EmittedBy != "zhen-cli" {
		t.Errorf("emitted_by: got %q want %q (audit trail must distinguish CLI vs web)", got.EmittedBy, "zhen-cli")
	}
	if len(got.Args) != 0 {
		t.Errorf("args: got %+v want empty", got.Args)
	}
}

// TestRunbookShow_PassesName asserts /runbook show <name> threads the
// name into args without mutating it.
func TestRunbookShow_PassesName(t *testing.T) {
	f := newFakeAgentd(t, func(req toolExecRequest) toolExecResponse {
		return toolExecResponse{Status: "ok", Result: "runbook body"}
	})
	s := newTestSession(f.srv.URL)
	s.handleRunbook([]string{"show", "observe/service-health-sweep"})

	if len(f.captured) != 1 {
		t.Fatalf("got %d requests", len(f.captured))
	}
	got := f.captured[0]
	if got.Tool != "runbook_show" {
		t.Errorf("tool: got %q want %q", got.Tool, "runbook_show")
	}
	if name, ok := got.Args["name"].(string); !ok || name != "observe/service-health-sweep" {
		t.Errorf("args.name: got %v want %q", got.Args["name"], "observe/service-health-sweep")
	}
}

// TestRunbookExec_BareNameDispatchesAsExecute asserts the shorthand
// `/runbook <name>` (without a subcommand) is routed to runbook_execute
// — the mutating tool — so Champion's full three-rule gate runs. This
// is the T6b closure regression test: the CLI must NEVER have a
// non-/api/v1/tool/exec path that runs runbooks. If a future refactor
// adds a direct-exec shortcut, this test will catch it because there
// would be 0 captured requests.
func TestRunbookExec_BareNameDispatchesAsExecute(t *testing.T) {
	f := newFakeAgentd(t, func(req toolExecRequest) toolExecResponse {
		return toolExecResponse{Status: "ok", Result: nil}
	})
	s := newTestSession(f.srv.URL)
	s.handleRunbook([]string{"observe/service-health-sweep"})

	if len(f.captured) != 1 {
		t.Fatalf("T6b regression: expected 1 /api/v1/tool/exec call, got %d (would mean a bypass path)", len(f.captured))
	}
	got := f.captured[0]
	if got.Tool != "runbook_execute" {
		t.Errorf("bare name should route to runbook_execute; got %q", got.Tool)
	}
	if dr, ok := got.Args["dry_run"].(bool); !ok || dr {
		t.Errorf("dry_run: got %v want false (real exec)", got.Args["dry_run"])
	}
}

// TestRunbookExec_DenialIsHandled asserts a denied response from the
// daemon does NOT raise a panic and is rendered as an error.
func TestRunbookExec_DenialIsHandled(t *testing.T) {
	f := newFakeAgentd(t, func(req toolExecRequest) toolExecResponse {
		return toolExecResponse{Status: "denied", Reason: "Champion rule 2: untrusted justification"}
	})
	s := newTestSession(f.srv.URL)
	// Should not panic; output goes to stderr (not asserted here — covered
	// by the broader rendering tests in handleRunbook).
	s.handleRunbook([]string{"exec", "destructive/wipe-everything"})
}

// TestRunbookExec_PendingConfirmationIsSurfaced asserts the
// pending_confirmation status path threads token + reason without
// auto-confirming.
func TestRunbookExec_PendingConfirmationIsSurfaced(t *testing.T) {
	f := newFakeAgentd(t, func(req toolExecRequest) toolExecResponse {
		return toolExecResponse{
			Status:       "pending_confirmation",
			Reason:       "novel mutation requires explicit confirm",
			PendingToken: "tok-abc-123",
		}
	})
	s := newTestSession(f.srv.URL)
	s.handleRunbook([]string{"exec", "infra/restart-wotan"})
	// The CLI must NOT auto-confirm by hitting /api/v1/agent/confirm —
	// only one request should be captured.
	if len(f.captured) != 1 {
		t.Fatalf("auto-confirm regression: %d requests captured", len(f.captured))
	}
}

// TestRecallRemember_AreDeferred asserts the Phase 2b deferral does
// not silently dispatch to /api/v1/tool/exec (which would risk
// Champion classifying an unknown tool as mutating + denying).
func TestRecallRemember_AreDeferred(t *testing.T) {
	f := newFakeAgentd(t, func(req toolExecRequest) toolExecResponse {
		t.Errorf("/recall and /remember must NOT call /api/v1/tool/exec — Phase 2b deferred")
		return toolExecResponse{Status: "error"}
	})
	s := newTestSession(f.srv.URL)
	if s.handleSlash("/recall foo") {
		t.Error("/recall should not terminate REPL")
	}
	if s.handleSlash("/remember") {
		t.Error("/remember should not terminate REPL")
	}
	if len(f.captured) != 0 {
		t.Errorf("/recall and /remember leaked through to tool/exec: %d requests", len(f.captured))
	}
}

// TestToolExec_DecodesValidResponseShapes covers all the documented
// status values from cmd/zhen-agentd/toolexec.go.
func TestToolExec_DecodesValidResponseShapes(t *testing.T) {
	cases := []struct {
		name   string
		status string
	}{
		{"ok", "ok"},
		{"denied", "denied"},
		{"pending_confirmation", "pending_confirmation"},
		{"unknown_tool", "unknown"},
		{"used_token", "used"},
		{"expired_token", "expired"},
		{"error_path", "error"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			f := newFakeAgentd(t, func(req toolExecRequest) toolExecResponse {
				return toolExecResponse{Status: c.status, Reason: "test"}
			})
			s := newTestSession(f.srv.URL)
			resp, err := s.toolExec("noop", map[string]any{})
			if err != nil {
				t.Fatalf("toolExec: %v", err)
			}
			if resp.Status != c.status {
				t.Errorf("status round-trip: got %q want %q", resp.Status, c.status)
			}
		})
	}
}

// TestToolExec_DaemonUnreachableErrors asserts a clear error when
// zhen-agentd is down (degraded-mode signal for Phase 3 fallback work).
func TestToolExec_DaemonUnreachableErrors(t *testing.T) {
	// Use an invalid URL that the http client cannot reach.
	s := newTestSession("http://127.0.0.1:1") // port 1 is universally closed
	_, err := s.toolExec("runbook_list", nil)
	if err == nil {
		t.Fatal("expected unreachable error, got nil")
	}
	if !strings.Contains(err.Error(), "zhen-agentd unreachable") {
		t.Errorf("error message lost the daemon-unreachable hint: %v", err)
	}
}
