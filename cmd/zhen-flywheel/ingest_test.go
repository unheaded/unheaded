// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/zhencapture"
)

// funcScanner flags the kinds whose marker appears in the text.
type funcScanner map[string]string // marker -> kind

func (f funcScanner) Scan(_ context.Context, s string) (map[string]int, error) {
	out := map[string]int{}
	for marker, kind := range f {
		out[kind] += strings.Count(s, marker)
	}
	return out, nil
}

var testScanner = funcScanner{"PLANTED-SECRET": "secret", "alice@corp-internal.net": "email"}

// captureDir is a private capture directory outside any repository.
func captureDir(t *testing.T) string {
	t.Helper()
	d := filepath.Join(t.TempDir(), "zhen-capture")
	if err := os.MkdirAll(d, 0o700); err != nil {
		t.Fatal(err)
	}
	return d
}

// writeRecords writes records through the real writer, so the format is
// whatever pkg/zhencapture produces.
func writeRecords(t *testing.T, dir string, recs ...*zhencapture.Record) string {
	t.Helper()
	w, err := zhencapture.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recs {
		if r.Time.IsZero() {
			r.Time = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
		}
		if err := w.Write(r); err != nil {
			t.Fatal(err)
		}
	}
	files, _ := filepath.Glob(filepath.Join(dir, "zhen-capture-*.jsonl"))
	if len(files) != 1 {
		t.Fatalf("capture files = %v", files)
	}
	return files[0]
}

func appendRaw(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatal(err)
	}
}

func localRecord(prompt, answer string) *zhencapture.Record {
	return &zhencapture.Record{
		Source: "zhen-claude", Route: zhencapture.RouteLocal, Prompt: prompt, ConversationID: "conv-" + prompt,
		TaskFamily: "repo-task", Local: &zhencapture.Leg{Model: "qwen-coder-14b", Answer: answer, StopReason: "stop"},
		Final: answer, AcceptedBy: "local",
	}
}

func escalatedRecord() *zhencapture.Record {
	return &zhencapture.Record{
		Source: "zhen-claude", Route: zhencapture.RouteEscalated, Prompt: "fix the race", ConversationID: "claude-conv-1",
		Local:  &zhencapture.Leg{Model: "qwen-coder-14b", Answer: "local draft", StopReason: "stop"},
		Check:  &zhencapture.Check{Argv: []string{"go", "test", "-race"}, Exit: 1},
		Claude: &zhencapture.Leg{Model: "claude-opus-5-5", Answer: "claude fix", StopReason: "end_turn"},
		ToolCalls: []zhencapture.ToolCall{{Name: "read_file", Decision: "allowed",
			Input: json.RawMessage(`{"path":"a.go"}`), Output: "package a"}},
		Final: "claude fix", AcceptedBy: "claude",
	}
}

func newIngester(st Store) *Ingester {
	return &Ingester{Store: st, Scanner: testScanner, Log: &bytes.Buffer{}}
}

func TestIngest_StrictLinesReportedNeverGuessed(t *testing.T) {
	dir := captureDir(t)
	f := writeRecords(t, dir, localRecord("a", "answer a"))
	appendRaw(t, f, `{"schema":"zhen-capture/v1","id":"x","prompt":`) // line 2: malformed
	appendRaw(t, f, `{"schema":"zhen-capture/v2","id":"y","ts":"2026-10-23T12:00:00Z","source":"zhen-claude","prompt":"p","local":{"model":"m","answer":"a"}}`)
	appendRaw(t, f, `{"schema":"zhen-capture/v1","id":"z","ts":"2026-10-23T12:00:00Z","source":"zhen-claude","prompt":"p","task_family":"Bad Family","local":{"model":"m","answer":"a"}}`)
	appendRaw(t, f, `{"schema":"zhen-capture/v1","id":"w","ts":"2026-10-23T12:00:00Z","source":"zhen-claude","prompt":"p","local":{"model":"m","answer":"a","latency_ms":"slow"}}`)
	appendRaw(t, f, "")
	w, _ := zhencapture.Open(dir)
	if err := w.Write(localRecord("b", "answer b")); err != nil { // line 7 (6 is blank)
		t.Fatal(err)
	}
	st := newMemStore()
	rep, err := newIngester(st).IngestDir(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Inserted != 2 || len(st.captures) != 2 {
		t.Errorf("inserted %d, store %d; want the 2 good lines", rep.Inserted, len(st.captures))
	}
	want := []string{":2:", ":3:", ":4:", ":5:"}
	if len(rep.Errors) != len(want) {
		t.Fatalf("errors = %q", rep.Errors)
	}
	for i, w := range want {
		if !strings.Contains(rep.Errors[i], w) {
			t.Errorf("error %d = %q, want line %s", i, rep.Errors[i], w)
		}
	}
	if !strings.Contains(rep.Errors[1], "schema") || !strings.Contains(rep.Errors[2], "task_family") {
		t.Errorf("errors do not say why: %q", rep.Errors)
	}
	if _, err := os.Stat(f); err != nil {
		t.Error("a file with bad lines must stay where it is, not move to ingested/")
	}
}

func TestIngest_RefusesLooseFileAndRepoDir(t *testing.T) {
	dir := captureDir(t)
	f := writeRecords(t, dir, localRecord("a", "answer a"))
	if err := os.Chmod(f, 0o644); err != nil {
		t.Fatal(err)
	}
	st := newMemStore()
	rep, err := newIngester(st).IngestDir(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.captures) != 0 || len(rep.Errors) != 1 || !strings.Contains(rep.Errors[0], "0644") {
		t.Errorf("a 0644 capture file was read: store %d, errors %q", len(st.captures), rep.Errors)
	}

	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(repo, "captures")
	if err := os.Mkdir(inside, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := newIngester(st).IngestDir(context.Background(), inside); err == nil || !strings.Contains(err.Error(), "git work tree") {
		t.Errorf("capture dir inside a repository: err = %v", err)
	}
}

func TestIngest_RescanQuarantines(t *testing.T) {
	dir := captureDir(t)
	writeRecords(t, dir,
		localRecord("leak", "the key is PLANTED-SECRET ok"),
		localRecord("mail", "write to alice@corp-internal.net"),
		localRecord("clean", "nothing to see"))
	st := newMemStore()
	rep, err := newIngester(st).IngestDir(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Quarantined != 2 || rep.Inserted != 1 || len(rep.Errors) != 0 {
		t.Fatalf("report = %+v", rep)
	}
	for _, c := range st.captures {
		if strings.Contains(c.Response, "PLANTED") || strings.Contains(c.Response, "alice@") {
			t.Errorf("quarantined text reached the Well: %q", c.Response)
		}
	}
	qs, _ := filepath.Glob(filepath.Join(dir, "quarantine", "*.jsonl"))
	if len(qs) != 1 {
		t.Fatalf("quarantine files = %v", qs)
	}
	fi, _ := os.Stat(qs[0])
	di, _ := os.Stat(filepath.Join(dir, "quarantine"))
	if fi.Mode().Perm() != 0o600 || di.Mode().Perm() != 0o700 {
		t.Errorf("quarantine modes file %o dir %o", fi.Mode().Perm(), di.Mode().Perm())
	}
	raw, _ := os.ReadFile(qs[0])
	if n := strings.Count(string(raw), "\n"); n != 2 || !strings.Contains(string(raw), `"kinds"`) {
		t.Errorf("quarantine file: %d lines, %s", n, raw)
	}
	for _, r := range rep.QuarantineReasons {
		if strings.Contains(r, "PLANTED") || strings.Contains(r, "alice") {
			t.Errorf("a reason carries the value: %q", r)
		}
	}
}

func TestIngest_IdempotentAndMovesFiles(t *testing.T) {
	dir := captureDir(t)
	f := writeRecords(t, dir, localRecord("a", "answer a"), escalatedRecord())
	saved, _ := os.ReadFile(f)
	st := newMemStore()
	rep, err := newIngester(st).IngestDir(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Inserted != 3 || rep.LabelsInserted != 2 {
		t.Fatalf("first ingest: %+v", rep)
	}
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Error("ingested file still in the capture dir")
	}
	moved := filepath.Join(dir, "ingested", filepath.Base(f))
	if fi, err := os.Stat(moved); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("not moved to ingested/: %v", err)
	}
	// The same file again (restored by hand): nothing new.
	if err := os.WriteFile(f, saved, 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err = newIngester(st).IngestDir(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Inserted != 0 || rep.Duplicates != 3 || rep.LabelsInserted != 0 || len(st.captures) != 3 {
		t.Errorf("second ingest: %+v (store %d)", rep, len(st.captures))
	}
	if ing, _ := filepath.Glob(filepath.Join(dir, "ingested", "*")); len(ing) != 2 {
		t.Errorf("ingested/ = %v; the second copy must not overwrite the first", ing)
	}
}

func TestCapturesFromRecord(t *testing.T) {
	dir := captureDir(t)
	writeRecords(t, dir, escalatedRecord())
	st := newMemStore()
	if _, err := newIngester(st).IngestDir(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	var local, claude Capture
	for _, c := range st.captures {
		switch c.AuthorKind {
		case AuthorLocal:
			local = c
		case AuthorClaude:
			claude = c
		}
	}
	if local.Response != "local draft" || local.Backend != "local" || local.Model != "qwen-coder-14b" || string(local.ToolTrace) != "[]" {
		t.Errorf("local capture = %+v", local)
	}
	if claude.Response != "claude fix" || claude.Backend != "claude" || !strings.Contains(string(claude.ToolTrace), `"read_file"`) {
		t.Errorf("claude capture = %+v", claude)
	}
	for _, c := range []Capture{local, claude} {
		if c.ConversationID != "claude-conv-1" || c.TaskFamily != UnknownFamily || c.Source != SourceZhenClaude ||
			c.Schema != zhencapture.Schema || c.RedactionVersion != RedactionVersion || c.RecordID == "" {
			t.Errorf("provenance = %+v", c)
		}
		id, _ := CaptureID(c.Model, c.Prompt, c.Response, c.ToolTrace)
		if c.ID != id {
			t.Errorf("id %s is not the content hash %s", c.ID, id)
		}
	}
	if got := st.labelsOf(local.ID); got[LabelCheckFail] != 1 || got[LabelEscalated] != 1 || len(got) != 2 {
		t.Errorf("local labels = %v", got)
	}
	if got := st.labelsOf(claude.ID); len(got) != 0 {
		t.Errorf("claude labels = %v (its check is not recorded)", got)
	}
}

func TestCaptureIDCanonical(t *testing.T) {
	a, _ := CaptureID("m", "p", "r", json.RawMessage(`[{"b":1,"a":2}]`))
	b, _ := CaptureID("m", "p", "r", json.RawMessage(` [ {"a":2, "b":1} ] `))
	c, _ := CaptureID("m", "p", "r2", nil)
	d, _ := CaptureID("m", "p", "r2", json.RawMessage(`[]`))
	if a != b || c != d || a == c || !captureRE.MatchString(a) {
		t.Errorf("ids: %s %s %s %s", a, b, c, d)
	}
	if _, err := CaptureID("m", "p", "r", json.RawMessage(`{"not":"array"}`)); err == nil {
		t.Error("a non-array trace was accepted")
	}
}

func TestIngest_HumanAndTruncated(t *testing.T) {
	dir := captureDir(t)
	human := &zhencapture.Record{Source: "zhen-agentd", Route: zhencapture.RouteLocal, Prompt: "card", RunID: "0123456789abcdef",
		TaskFamily: "runbook-draft", AuthorKind: zhencapture.AuthorHuman, Final: "Stevie's version"}
	cut := localRecord("long", "half an answ")
	cut.Local.StopReason = "length"
	writeRecords(t, dir, human, cut)
	st := newMemStore()
	if _, err := newIngester(st).IngestDir(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	var sawHuman, sawCut bool
	for _, c := range st.captures {
		if c.AuthorKind == AuthorHuman {
			sawHuman = c.Backend == BackendNone && c.Response == "Stevie's version" && c.RunID == "0123456789abcdef" && c.Source == SourceAgentd
		}
		if c.Response == "half an answ" {
			sawCut = c.Truncated
		}
	}
	if !sawHuman || !sawCut {
		t.Errorf("human %v truncated %v: %+v", sawHuman, sawCut, st.captures)
	}
}
