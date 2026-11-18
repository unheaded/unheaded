// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"unheaded/pkg/auth"
	"unheaded/pkg/modules"
	"unheaded/pkg/zhenagent"
)

// dirWithZhen points pkg/modules at a fresh directory holding zhen/: the
// Zhen module is on for this test (ADR-149).
func dirWithZhen(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "zhen"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(modules.EnvDir, dir)
	return dir
}

// zhenOff points pkg/modules at an empty directory: no module is on.
func zhenOff(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(modules.EnvDir, dir)
	return dir
}

// panicRunStore fails the test if anything asks it for an agent run.
type panicRunStore struct {
	*optInStore
	t *testing.T
}

func (p panicRunStore) LatestAgentRun(context.Context, string) (*AgentRun, error) {
	p.t.Error("LatestAgentRun called while the Zhen module is off")
	return nil, nil
}

func detailKeys(t *testing.T, s *Server, id string) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	s.handleTaskByID(rec, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+id, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", id, rec.Code, rec.Body)
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestTaskDetail_NoZhenFieldsWhileTheModuleIsOff(t *testing.T) {
	zhenOff(t)
	s, st := optInServer()
	st.rows["card-1"] = ZhenOptIn{CardSHA256: zhenagent.CardSHA256("t", "do the thing"), By: "stevie", At: time.Now()}
	s.store = panicRunStore{optInStore: st, t: t}
	m := detailKeys(t, s, "card-1")
	if _, ok := m["zhen_opt_in"]; ok {
		t.Error("zhen_opt_in in the detail with the module off")
	}
	if _, ok := m["agent_run"]; ok {
		t.Error("agent_run in the detail with the module off")
	}
	if _, ok := m["zhen_module"]; ok {
		t.Error("zhen_module in the detail with the module off")
	}
}

func TestZhenOptIn_RefusedWhileTheModuleIsOff(t *testing.T) {
	zhenOff(t)
	s, st := optInServer()
	rec := optInCall(s, &auth.Identity{Subject: "stevie", Roles: []string{auth.RoleOperator}}, http.MethodPost, "card-1")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("%d, want 404", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] != "the zhen module is off" {
		t.Fatalf("body = %s", rec.Body)
	}
	if len(st.rows) != 0 {
		t.Fatal("the store was written")
	}
}

func TestAgentRun_NoQueryNoWarningWhileTheModuleIsOff(t *testing.T) {
	zhenOff(t)
	was := agentRunWarned.Swap(false)
	t.Cleanup(func() { agentRunWarned.Store(was) })
	s, st := optInServer()
	s.store = panicRunStore{optInStore: st, t: t}
	if run := s.agentRunFor(context.Background(), "card-1"); run != nil {
		t.Fatalf("run = %+v", run)
	}
	if agentRunWarned.Load() {
		t.Fatal("a warning about zhen_agent_runs with the module off")
	}
}

func TestZhenFieldsReturnWithTheModule(t *testing.T) {
	dirWithZhen(t)
	s, st := optInServer()
	st.rows["card-1"] = ZhenOptIn{CardSHA256: zhenagent.CardSHA256("t", "do the thing"), By: "stevie", At: time.Now()}
	m := detailKeys(t, s, "card-1")
	if _, ok := m["zhen_opt_in"]; !ok {
		t.Fatal("zhen_opt_in missing with the module on")
	}
	if m["zhen_module"] != true {
		t.Fatal("zhen_module not true with the module on")
	}
}
