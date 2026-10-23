// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build wellint

package main

// Runs only under scripts/well/test-zhen-captures.sh --with-ingest,
// against a throwaway server, as app_zhen (WELL_TEST_APP_DSN). Never
// the real Well.

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestWellStore_IngestLabelRoundTrip(t *testing.T) {
	dsn := os.Getenv("WELL_TEST_APP_DSN")
	if dsn == "" {
		t.Skip("WELL_TEST_APP_DSN unset (run scripts/well/test-zhen-captures.sh --with-ingest)")
	}
	st, err := openPG(dsn, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	ctx := context.Background()

	dir := captureDir(t)
	f := writeRecords(t, dir, localRecord("well-a", "answer é \"quoted\" 3.50"), escalatedRecord())
	saved, _ := os.ReadFile(f)
	rep, err := (&Ingester{Store: st, Scanner: testScanner}).IngestDir(ctx, dir)
	if err != nil || rep.Inserted != 3 || rep.LabelsInserted != 2 || len(rep.Errors) != 0 {
		t.Fatalf("ingest: %+v %v", rep, err)
	}
	if err := os.WriteFile(f, saved, 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err = (&Ingester{Store: st, Scanner: testScanner}).IngestDir(ctx, dir)
	if err != nil || rep.Inserted != 0 || rep.Duplicates != 3 || rep.LabelsInserted != 0 {
		t.Fatalf("re-ingest: %+v %v", rep, err)
	}

	caps, err := st.Captures(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byResp := map[string]Capture{}
	for _, c := range caps {
		byResp[c.Response] = c
	}
	a, ok := byResp["answer é \"quoted\" 3.50"]
	if !ok {
		t.Fatalf("captures = %+v", caps)
	}
	if id, _ := CaptureID(a.Model, a.Prompt, a.Response, a.ToolTrace); id != a.ID {
		t.Errorf("the stored row no longer hashes to its id: %s vs %s", id, a.ID)
	}
	cl := byResp["claude fix"]
	if id, _ := CaptureID(cl.Model, cl.Prompt, cl.Response, cl.ToolTrace); id != cl.ID {
		t.Errorf("tool trace did not survive the JSONB round trip: %s vs %s (%s)", id, cl.ID, cl.ToolTrace)
	}
	labels, err := st.Labels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mine := 0
	for _, l := range labels {
		if l.CaptureID == byResp["local draft"].ID {
			mine++
		}
	}
	if mine != 2 {
		t.Fatalf("labels on the escalated local capture = %d of %+v", mine, labels)
	}
	if _, err := st.InsertLabel(ctx, Label{CaptureID: "f" + a.ID[1:], Kind: LabelJudgePass, SourceRef: "x"}); !errors.Is(err, errMissingCapture) {
		t.Errorf("label on a missing capture: %v", err)
	}

	// zhen_actions: the run_check rows label reads.
	if _, err := st.app.ExecContext(ctx, `INSERT INTO zhen_actions (session_id, action_type, status, intent, result_summary)
		VALUES ('conv-well-a', 'run_check', 'completed', 'run_check go-test ./...', 'exit=0 pass=true')`); err != nil {
		t.Fatal(err)
	}
	checks, err := st.CheckActions(ctx)
	if err != nil || len(checks) != 1 || checks[0].SessionID != "conv-well-a" || checks[0].Summary != "exit=0 pass=true" {
		t.Errorf("checks = %+v %v", checks, err)
	}
	// The status pkg/champion used for a failing check before this sprint
	// is not allowed by 009's CHECK: the update failed silently and the
	// row stayed 'planned'.
	if _, err := st.app.ExecContext(ctx, `UPDATE zhen_actions SET status = 'check_failed' WHERE session_id = 'conv-well-a'`); err == nil {
		t.Error("zhen_actions accepted status check_failed; the runcheck.go fix assumed it would not")
	}

	if _, err := st.Escalations(ctx); !errors.Is(err, ErrUnavailable) {
		t.Errorf("escalations without WELL_OPS_DSN: %v", err)
	}
	if _, err := st.AgentRuns(ctx); !errors.Is(err, ErrUnavailable) {
		t.Errorf("agent runs before 032: %v", err)
	}

	if err := st.Promote(ctx, []string{a.ID}); err != nil {
		t.Fatal(err)
	}
	if ok, err := st.Quarantine(ctx, cl.ID); err != nil || !ok {
		t.Fatalf("quarantine: %v %v", ok, err)
	}
	caps, _ = st.Captures(ctx)
	for _, c := range caps {
		if c.ID == a.ID && !c.Promoted {
			t.Error("promote did not stick")
		}
		if c.ID == cl.ID && !c.Quarantined {
			t.Error("quarantine did not stick")
		}
	}
}
