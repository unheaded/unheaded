// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"unheaded/pkg/champion"
)

// ADR-060 (Consequences, Negative): zhen-agentd is coupled to
// scripts/switch-model.sh's output; the mitigation is the script's --json
// flag plus a golden parse at the Go layer. The samples in testdata/ were
// recorded from the real script on west (2026-10-27); when the script's
// JSON changes, re-record them:
//
//	scripts/switch-model.sh --json --status    > testdata/switch-model-status.json
//	scripts/switch-model.sh --json --check qwen-7b > testdata/switch-model-check.json
func TestModelSwapJSON_GoldenStatus(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "switch-model-status.json"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := champion.ParseModelStatus(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(st.Models) < 2 {
		t.Fatalf("models %+v, want the script's keys", st.Models)
	}
	seen := map[string]champion.ModelInfo{}
	for _, m := range st.Models {
		seen[m.Key] = m
	}
	q, ok := seen["qwen-7b"]
	if !ok || q.File == "" || q.Name == "" || q.LoadSeconds <= 0 {
		t.Fatalf("qwen-7b entry %+v: want file, name and load_seconds", q)
	}
	if st.Port != 8081 {
		t.Errorf("port %d, want 8081", st.Port)
	}
	if st.Fit != nil {
		t.Errorf("--status carries a fit verdict: %+v", st.Fit)
	}
}

func TestModelSwapJSON_GoldenCheck(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "switch-model-check.json"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := champion.ParseModelStatus(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if st.Fit == nil || st.Fit.Key != "qwen-7b" || (st.Fit.Verdict != "fits" && st.Fit.Verdict != "refused") {
		t.Fatalf("fit %+v, want a qwen-7b verdict", st.Fit)
	}
	if st.Fit.NeedMiB <= 0 || st.Fit.Offload == "" {
		t.Errorf("fit numbers missing: %+v", st.Fit)
	}
}

func TestModelSwapJSON_RejectsGarbage(t *testing.T) {
	for _, in := range []string{"", "[fake] swapped", `{"models":"x"}`, `{"active":null}`} {
		if _, err := champion.ParseModelStatus([]byte(in)); err == nil {
			t.Errorf("%q parsed", in)
		}
	}
}

// GET /api/v1/models runs the daemon root's switch-model.sh --json --status
// and returns what it parsed (zhen-cli /model lists from here).
func TestModels_ListsFromScriptJSON(t *testing.T) {
	root := t.TempDir()
	golden, err := os.ReadFile(filepath.Join("testdata", "switch-model-status.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "status.json"), golden, 0o644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/bash\n[ \"$1 $2\" = \"--json --status\" ] || { echo \"bad args: $*\" >&2; exit 2; }\ncat \"$(dirname \"$0\")/status.json\"\n"
	if err := os.WriteFile(filepath.Join(root, "scripts", "switch-model.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	pool := newChampionPool(newMetricsActionStore(nopActionStore{}), nil)
	srv := &server{pool: pool, defaultRoot: root, allowed: map[string]struct{}{root: {}}}
	rec := as("stevie", srv.handleModels, http.MethodGet, "/api/v1/models", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("models: %d %s", rec.Code, rec.Body.String())
	}
	var st champion.ModelStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if len(st.Models) < 2 {
		t.Fatalf("models %+v", st.Models)
	}
	if rec := as("stevie", srv.handleModels, http.MethodPost, "/api/v1/models", nil); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d, want 405", rec.Code)
	}
}
