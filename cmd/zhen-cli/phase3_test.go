// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// ADR-059 Phases 2b and 3 (S-CHAMPION-HARDENING Steps 209, 213): /recall,
// /remember, /confirm and /model all go through zhen-agentd. The CLI holds
// no database driver and never runs switch-model.sh itself.
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type capturedReq struct {
	Method, Path, Query string
	Bearer, APIKey      string
	Body                map[string]any
}

// agentdStub answers each path with a canned JSON body and records requests.
type agentdStub struct {
	mu   sync.Mutex
	reqs []capturedReq
	srv  *httptest.Server
}

func newAgentdStub(t *testing.T, routes map[string]func(capturedReq) (int, any)) *agentdStub {
	t.Helper()
	a := &agentdStub{}
	a.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		c := capturedReq{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery,
			Bearer: strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), APIKey: r.Header.Get("X-API-Key")}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &c.Body)
		}
		a.mu.Lock()
		a.reqs = append(a.reqs, c)
		a.mu.Unlock()
		h, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		code, body := h(c)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(a.srv.Close)
	return a
}

func stubSession(url string) (*session, *bytes.Buffer) {
	s := newTestSession(url)
	var out bytes.Buffer
	s.out = &out
	s.errOut = &out
	s.agentdKey = "k-test"
	return s, &out
}

func TestRecall_PrintsRowsWithProvenance(t *testing.T) {
	a := newAgentdStub(t, map[string]func(capturedReq) (int, any){
		"/api/v1/memory/recall": func(capturedReq) (int, any) {
			return 200, map[string]any{"memories": []map[string]any{
				{"id": 7, "question": "how do I list runbooks?", "answer": "/runbook list", "source": "ui:stevie",
					"model": "qwen-7b", "created_at": "2026-10-27T20:00:00Z", "score": 0.42},
			}}
		},
	})
	s, out := stubSession(a.srv.URL)
	s.handleSlash("/recall list runbooks")
	if len(a.reqs) != 1 || a.reqs[0].Method != http.MethodGet || !strings.Contains(a.reqs[0].Query, "q=list+runbooks") {
		t.Fatalf("requests %+v", a.reqs)
	}
	for _, want := range []string{"#7", "ui:stevie", "qwen-7b", "2026-10-27", "how do I list runbooks?", "/runbook list"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
	if a.reqs[0].Bearer != "k-test" || a.reqs[0].APIKey != "k-test" {
		t.Errorf("auth headers %+v", a.reqs[0])
	}
}

func TestRemember_PostsAndPrintsID(t *testing.T) {
	a := newAgentdStub(t, map[string]func(capturedReq) (int, any){
		"/api/v1/memory/remember": func(capturedReq) (int, any) {
			return 200, map[string]any{"status": "ok", "result": map[string]any{"id": 42}}
		},
	})
	s, out := stubSession(a.srv.URL)
	s.model = "qwen2.5-coder-7b-instruct"

	// No argument and no exchange yet: nothing to remember, no request.
	s.handleSlash("/remember")
	if len(a.reqs) != 0 {
		t.Fatalf("remembered with an empty history: %+v", a.reqs)
	}

	// No argument: the last exchange.
	s.history = []chatMessage{{Role: "user", Content: "q1"}, {Role: "assistant", Content: "a1"}}
	s.handleSlash("/remember")
	// With text: a note.
	s.handleSlash("/remember west runs llama-server on 8081")
	if len(a.reqs) != 2 {
		t.Fatalf("requests %+v", a.reqs)
	}
	if b := a.reqs[0].Body; b["question"] != "q1" || b["answer"] != "a1" || b["model"] != s.model {
		t.Errorf("last-exchange body %+v", b)
	}
	if b := a.reqs[1].Body; b["answer"] != "west runs llama-server on 8081" || b["question"] == "" {
		t.Errorf("note body %+v", b)
	}
	if !strings.Contains(out.String(), "#42") {
		t.Errorf("output lacks the stored id:\n%s", out.String())
	}
}

func TestConfirm_PostsTokenAndPrintsResult(t *testing.T) {
	a := newAgentdStub(t, map[string]func(capturedReq) (int, any){
		"/api/v1/agent/confirm": func(c capturedReq) (int, any) {
			if c.Body["token"] == "tok-ok" {
				return 200, map[string]any{"status": "ok", "result": "ran observe/health-sweep"}
			}
			return 403, map[string]any{"status": "denied", "reason": "token bound to another principal"}
		},
	})
	s, out := stubSession(a.srv.URL)
	s.handleSlash("/confirm tok-ok")
	s.handleSlash("/confirm tok-other")
	s.handleSlash("/confirm")
	if len(a.reqs) != 2 {
		t.Fatalf("requests %+v (a bare /confirm must not post)", a.reqs)
	}
	if a.reqs[0].Method != http.MethodPost || a.reqs[0].Bearer != "k-test" {
		t.Errorf("request %+v", a.reqs[0])
	}
	for _, want := range []string{"ran observe/health-sweep", "token bound to another principal"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
}

// A pending confirmation now names the command that redeems it.
func TestPendingConfirmation_PointsAtConfirm(t *testing.T) {
	a := newAgentdStub(t, map[string]func(capturedReq) (int, any){
		"/api/v1/tool/exec": func(capturedReq) (int, any) {
			return 403, map[string]any{"status": "pending_confirmation", "pending_token": "tok-9", "reason": "rule 2"}
		},
	})
	s, out := stubSession(a.srv.URL)
	s.handleRunbook([]string{"exec", "infra/restart-wotan"})
	if !strings.Contains(out.String(), "/confirm tok-9") {
		t.Errorf("output does not say how to confirm:\n%s", out.String())
	}
}

func TestModel_ListsAndSwitchesThroughAgentd(t *testing.T) {
	a := newAgentdStub(t, map[string]func(capturedReq) (int, any){
		"/api/v1/models": func(capturedReq) (int, any) {
			return 200, map[string]any{"active": "qwen-7b", "port": 8081, "models": []map[string]any{
				{"key": "qwen-7b", "name": "qwen2.5-coder-7b-instruct", "present": true, "pinned": true, "load_seconds": 45},
				{"key": "qwen-coder-14b", "name": "qwen2.5-coder-14b-instruct", "present": true, "pinned": true, "load_seconds": 120},
			}}
		},
		"/api/v1/tool/exec": func(c capturedReq) (int, any) {
			return 200, map[string]any{"status": "ok", "result": map[string]any{"new_model": "qwen-coder-14b", "load_seconds": 61.5}}
		},
	})
	s, out := stubSession(a.srv.URL)
	s.handleSlash("/model")
	if !strings.Contains(out.String(), "* qwen-7b") || !strings.Contains(out.String(), "qwen-coder-14b") {
		t.Errorf("listing:\n%s", out.String())
	}
	s.handleSlash("/model qwen-coder-14b")
	if len(a.reqs) != 2 {
		t.Fatalf("requests %+v", a.reqs)
	}
	sw := a.reqs[1]
	if sw.Path != "/api/v1/tool/exec" || sw.Body["tool"] != "model_switch" {
		t.Fatalf("switch request %+v, want tool/exec model_switch", sw)
	}
	if args, _ := sw.Body["args"].(map[string]any); args["key"] != "qwen-coder-14b" {
		t.Errorf("args %+v", sw.Body["args"])
	}
	// A key with shell in it never leaves the CLI.
	s.handleSlash("/model qwen-7b;id")
	if len(a.reqs) != 2 {
		t.Errorf("malformed key was sent: %+v", a.reqs[len(a.reqs)-1])
	}
}

func TestLoadAPIKey(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "zhen-cli.conf")

	// Nothing set: no key, no error.
	if k, err := loadAPIKey(func(string) string { return "" }, conf); err != nil || k != "" {
		t.Fatalf("unset: %q %v", k, err)
	}
	// The env wins over the file, and the legacy name still works.
	env := map[string]string{"ZHEN_API_KEY": "from-env", "ZHEN_AGENTD_API_KEY": "legacy"}
	if k, _ := loadAPIKey(func(n string) string { return env[n] }, conf); k != "from-env" {
		t.Errorf("env: %q", k)
	}
	delete(env, "ZHEN_API_KEY")
	if k, _ := loadAPIKey(func(n string) string { return env[n] }, conf); k != "legacy" {
		t.Errorf("legacy env: %q", k)
	}

	if err := os.WriteFile(conf, []byte("# zhen-cli\napi_key = from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if k, err := loadAPIKey(func(string) string { return "" }, conf); err != nil || k != "from-file" {
		t.Fatalf("0600 file: %q %v", k, err)
	}
	// Readable by group or others: refuse loudly, even when the env is set.
	if err := os.Chmod(conf, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadAPIKey(func(string) string { return "" }, conf); err == nil || !strings.Contains(err.Error(), "0600") {
		t.Errorf("0644 file: %v, want a refusal naming 0600", err)
	}
	if _, err := loadAPIKey(func(n string) string { return "x" }, conf); err == nil {
		t.Error("0644 file with env set: want a refusal")
	}
	// A file without api_key is an error, not a silent empty key.
	if err := os.WriteFile(conf, []byte("url = x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(conf, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadAPIKey(func(string) string { return "" }, conf); err == nil {
		t.Error("file without api_key: want an error")
	}
}
