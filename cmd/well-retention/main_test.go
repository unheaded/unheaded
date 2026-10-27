// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const goodClasses = `version: 1
ratified: false
classes:
  - name: zhen-conversations
    table: zhen_conversations
    action: purge
    online_days: 90
  - name: zhen-captures
    table: zhen_captures
    action: purge-function
    online_days: 180
  - name: kanban-soft-deleted
    table: kanban_tasks
    action: archive-purge
    online_days: 90
  - name: audit-events
    table: audit_events
    action: partition-check
    online_days: 90
  - name: zhen-agent-runs
    table: zhen_agent_runs
    action: partition-check
`

func TestParseClassesAccepts(t *testing.T) {
	f, err := parseClasses(strings.NewReader(goodClasses))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if f.Ratified || len(f.Classes) != 5 {
		t.Fatalf("got ratified=%v classes=%d", f.Ratified, len(f.Classes))
	}
	if c := f.Classes[2]; c.Name != "kanban-soft-deleted" || c.Action != actionArchivePurge || c.OnlineDays != 90 {
		t.Fatalf("class 2: %+v", c)
	}
}

// The repo's own file is the one the timer runs: it must parse, audit and
// compliance history must never be deleting classes, and it stays
// unratified until Stevie says otherwise (E-C).
func TestRepoClassesFile(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "compliance", "retention", "classes.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := parseClasses(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("compliance/retention/classes.yaml: %v", err)
	}
	if len(f.Classes) == 0 {
		t.Fatal("no classes")
	}
	for _, c := range f.Classes {
		if (c.Table == "compliance_findings_history" || c.Table == "audit_events") && c.Action != actionPartitionCheck {
			t.Errorf("%s: %s must never be a deleting class", c.Name, c.Table)
		}
	}
}

func TestParseClassesRefuses(t *testing.T) {
	cases := map[string]string{
		"unknown field":      strings.Replace(goodClasses, "ratified: false", "ratified: false\nsql: DROP TABLE x", 1),
		"unknown table":      strings.Replace(goodClasses, "table: zhen_conversations", "table: pg_authid", 1),
		"action not allowed": strings.Replace(goodClasses, "table: audit_events\n    action: partition-check", "table: audit_events\n    action: purge", 1),
		"unknown action":     strings.Replace(goodClasses, "action: purge\n", "action: truncate\n", 1),
		"captures age drift": strings.Replace(goodClasses, "online_days: 180", "online_days: 365", 1),
		"zero age on purge":  strings.Replace(goodClasses, "action: purge\n    online_days: 90", "action: purge\n    online_days: 0", 1),
		"negative age":       strings.Replace(goodClasses, "online_days: 90\n  - name: audit", "online_days: -1\n  - name: audit", 1),
		"duplicate name":     goodClasses + "  - name: audit-events\n    table: llm_requests\n    action: partition-check\n",
		"table twice":        goodClasses + "  - name: audit-events-2\n    table: audit_events\n    action: partition-check\n",
		"bad name":           strings.Replace(goodClasses, "name: zhen-conversations", "name: Zhen Conversations", 1),
		"wrong version":      strings.Replace(goodClasses, "version: 1", "version: 2", 1),
		"hold on a check":    strings.Replace(goodClasses, "table: audit_events\n    action: partition-check", "table: audit_events\n    action: partition-check\n    hold: true", 1),
		"no classes":         "version: 1\nratified: false\nclasses: []\n",
		"absurd age":         strings.Replace(goodClasses, "online_days: 90\n  - name: zhen-captures", "online_days: 100000\n  - name: zhen-captures", 1),
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if src == goodClasses {
				t.Fatal("case did not change the fixture")
			}
			if _, err := parseClasses(strings.NewReader(src)); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

func TestSelectClasses(t *testing.T) {
	f, err := parseClasses(strings.NewReader(goodClasses))
	if err != nil {
		t.Fatal(err)
	}
	all, err := f.selectClasses("")
	if err != nil || len(all) != 5 {
		t.Fatalf("all: %d %v", len(all), err)
	}
	one, err := f.selectClasses("kanban-soft-deleted")
	if err != nil || len(one) != 1 || one[0].Table != "kanban_tasks" {
		t.Fatalf("one: %+v %v", one, err)
	}
	if _, err := f.selectClasses("nope"); err == nil {
		t.Fatal("unknown class accepted")
	}
}

func TestCutoff(t *testing.T) {
	now := time.Date(2026, 10, 27, 4, 10, 0, 0, time.UTC)
	if got := cutoff(now, 90); !got.Equal(time.Date(2026, 7, 29, 4, 10, 0, 0, time.UTC)) {
		t.Fatalf("cutoff 90: %v", got)
	}
	if got := cutoff(now, 0); !got.Equal(now) {
		t.Fatalf("cutoff 0: %v", got)
	}
}

func TestPartitionVerdict(t *testing.T) {
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	at := func(d int) *time.Time { x := now.Add(-time.Duration(d) * day); return &x }
	cases := []struct {
		name   string
		rows   int64
		bytes  int64
		oldest *time.Time
		days   int
		due    bool
		by     string
	}{
		{"empty table", 0, 8192, nil, 90, false, ""},
		{"young rows, short age", 15, 80 << 10, at(9), 90, true, "2026-10-18"},
		{"young rows, long age", 148, 288 << 10, at(12), 365, false, "2027-07-17"},
		{"rows leg", 1_000_000, 1 << 20, at(1), 365, true, "2027-07-28"},
		{"bytes leg", 10, 1 << 30, at(1), 365, true, "2027-07-28"},
		{"no age, small", 10, 1 << 20, at(400), 0, false, ""},
		{"no age, big", 2_000_000, 1 << 20, at(400), 0, true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := partitionVerdict(now, c.rows, c.bytes, c.oldest, c.days)
			if v.Due != c.due {
				t.Fatalf("due = %v (%s), want %v", v.Due, v.Reason, c.due)
			}
			got := ""
			if v.PartitionBy != nil {
				got = v.PartitionBy.Format("2006-01-02")
			}
			if got != c.by {
				t.Fatalf("partition_by = %q, want %q", got, c.by)
			}
			if v.Due && v.Reason == "" {
				t.Fatal("due without a reason")
			}
		})
	}
}

func TestRedactRow(t *testing.T) {
	key := "sk-ant-" + strings.Repeat("a1", 12)
	row := map[string]any{
		"id":          "card-1",
		"title":       "mail stevie@example.org about it",
		"description": "token " + key + " leaked",
		"progress":    float64(40),
		"commits":     []any{"abc", map[string]any{"note": "password=hunter22"}},
		"deleted_at":  nil,
	}
	out, n := redactRow(row)
	if n != 3 {
		t.Fatalf("redactions = %d, want 3: %v", n, out)
	}
	flat := strings.Join([]string{out["title"].(string), out["description"].(string),
		out["commits"].([]any)[1].(map[string]any)["note"].(string)}, " ")
	for _, leak := range []string{"stevie@example.org", key, "hunter22"} {
		if strings.Contains(flat, leak) {
			t.Fatalf("%q survived: %s", leak, flat)
		}
	}
	if out["id"] != "card-1" || out["progress"] != float64(40) || out["deleted_at"] != nil {
		t.Fatalf("non-secret fields changed: %v", out)
	}
	if row["title"] != "mail stevie@example.org about it" {
		t.Fatal("input row was modified in place")
	}
}

func TestPassword(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "pw")
	if err := os.WriteFile(p, []byte("from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if got, err := password(env(map[string]string{"WELL_PASSWORD": "from-env"})); err != nil || got != "from-env" {
		t.Fatalf("env: %q %v", got, err)
	}
	if got, err := password(env(map[string]string{"WELL_PASSWORD_FILE": p})); err != nil || got != "from-file" {
		t.Fatalf("file: %q %v", got, err)
	}
	if _, err := password(env(map[string]string{"WELL_PASSWORD": "a", "WELL_PASSWORD_FILE": p})); err == nil {
		t.Fatal("both set accepted")
	}
	if _, err := password(env(map[string]string{})); err == nil {
		t.Fatal("neither set accepted")
	}
	if _, err := password(env(map[string]string{"WELL_PASSWORD_FILE": filepath.Join(dir, "missing")})); err == nil {
		t.Fatal("missing file accepted")
	}
}

// A live run of an unratified policy is refused before any connection.
func TestRunRefusesLiveWhenUnratified(t *testing.T) {
	p := filepath.Join(t.TempDir(), "classes.yaml")
	if err := os.WriteFile(p, []byte(goodClasses), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WELL_PASSWORD", "x")
	t.Setenv("WELL_HOST", "203.0.113.1") // TEST-NET: a connection attempt would hang, not pass
	var out, errb bytes.Buffer
	code := run(context.Background(), []string{"-classes", p}, &out, &errb)
	if code != 2 || !strings.Contains(errb.String(), "ratified") {
		t.Fatalf("code %d, stderr %q", code, errb.String())
	}
}

func TestRunUsage(t *testing.T) {
	p := filepath.Join(t.TempDir(), "classes.yaml")
	if err := os.WriteFile(p, []byte(goodClasses), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string][]string{
		"unknown class": {"-classes", p, "-dry-run", "-class", "nope"},
		"missing file":  {"-classes", filepath.Join(t.TempDir(), "absent.yaml"), "-dry-run"},
		"stray arg":     {"-classes", p, "-dry-run", "extra"},
		"bad batch":     {"-classes", p, "-dry-run", "-batch", "0"},
	} {
		t.Run(name, func(t *testing.T) {
			var out, errb bytes.Buffer
			if code := run(context.Background(), args, &out, &errb); code != 2 {
				t.Fatalf("code %d, stderr %q", code, errb.String())
			}
		})
	}
}

func TestMetricsText(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	by := now.Add(48 * time.Hour)
	results := []result{
		{Class: "zhen-conversations", Action: actionPurge, Deleted: 126},
		{Class: "audit-events", Action: actionPartitionCheck, Table: "audit_events", Verdict: &verdict{Due: true, PartitionBy: &by}},
		{Class: "kanban-soft-deleted", Action: actionArchivePurge, Err: "boom"},
	}
	txt := metricsText(now, false, results)
	for _, want := range []string{
		"unheaded_well_retention_last_run_timestamp_seconds 1800000000",
		`unheaded_well_retention_rows_deleted{class="zhen-conversations"} 126`,
		`unheaded_well_retention_partition_due{class="audit-events",table="audit_events"} 1`,
		`unheaded_well_retention_class_failed{class="kanban-soft-deleted"} 1`,
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in:\n%s", want, txt)
		}
	}
	if strings.Contains(txt, "last_success") {
		t.Error("a run with a failed class must not report success")
	}
}
