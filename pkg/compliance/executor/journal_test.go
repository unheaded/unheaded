// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package executor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/pkg/compliance/remediation"
)

func readJournal(t *testing.T, e *env) []remediation.JournalEntry {
	t.Helper()
	f, err := os.Open(filepath.Join(e.state, journalName))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	entries, err := remediation.ReadJournal(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := remediation.VerifyChain(entries, remediation.GenesisHash); err != nil {
		t.Fatalf("journal does not verify: %v", err)
	}
	return entries
}

func TestJournalRecordsApplyRefusalAndUndo(t *testing.T) {
	e := newEnv(t)
	x := e.executor("east")
	dir, err := x.Apply(context.Background(), fix(t, "kmod-cis"))
	if err != nil {
		t.Fatal(err)
	}
	e.failing("sshd")
	if _, err := x.Apply(context.Background(), fix(t, "sshd-safe")); !errors.Is(err, ErrRefused) {
		t.Fatalf("want refusal, got %v", err)
	}
	if err := x.UndoDir(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	j := readJournal(t, e)
	if len(j) != 3 {
		t.Fatalf("%d entries, want apply, refused apply, undo", len(j))
	}
	want := []struct{ fix, action, outcome string }{
		{"kmod-cis", "apply", "success"}, {"sshd-safe", "apply", "refused"}, {"kmod-cis", "undo", "success"},
	}
	for i, w := range want {
		if j[i].FixID != w.fix || j[i].Action != w.action || j[i].Outcome != w.outcome || j[i].Host != "east" || j[i].Operator != "test" {
			t.Errorf("entry %d: %+v, want %+v", i+1, j[i], w)
		}
	}
	sum, _ := fix(t, "kmod-cis").SHA256()
	if j[0].FixSHA256 != sum || j[0].BackupDir != dir {
		t.Errorf("entry 1 does not name the fix content and backup: %+v", j[0])
	}
	if fi, _ := os.Stat(filepath.Join(e.state, journalName)); fi.Mode().Perm() != 0o600 {
		t.Errorf("journal mode %o", fi.Mode().Perm())
	}
}

func TestNoApplyOnABrokenJournal(t *testing.T) {
	e := newEnv(t)
	x := e.executor("east")
	if _, err := x.Apply(context.Background(), fix(t, "kmod-cis")); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(e.state, journalName)
	b, _ := os.ReadFile(p)
	if err := os.WriteFile(p, []byte(strings.Replace(string(b), `"success"`, `"failure"`, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	before := e.snapshot()
	_, err := x.Apply(context.Background(), fix(t, "banner-cis"))
	if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "journal") {
		t.Fatalf("got %v, want refusal over the journal", err)
	}
	if after := e.snapshot(); after != before {
		t.Error("changed files despite a broken journal")
	}
}

func TestUndoRefusesAnotherHostsBackup(t *testing.T) {
	e := newEnv(t)
	dir, err := e.executor("east").Apply(context.Background(), fix(t, "kmod-cis"))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.executor("west").UndoDir(context.Background(), dir); !errors.Is(err, ErrRefused) {
		t.Fatalf("got %v", err)
	}
}
