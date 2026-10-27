// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// RT-10 (S-CHAMPION-HARDENING Steps 60-61): an audit-store failure fails
// a mutation closed (ErrAuditUnavailable, nothing changes) and lets a
// read through, counted in champion_audit_write_failures_total.

type downStore struct {
	logErr, updateErr error
}

func (d downStore) LogAction(context.Context, *Action) (int64, error) {
	if d.logErr != nil {
		return 0, d.logErr
	}
	return 7, nil
}
func (d downStore) UpdateAction(context.Context, int64, string, string, string, int) error {
	return d.updateErr
}
func (d downStore) GetActions(context.Context, string, int) ([]Action, error) { return nil, nil }

func auditFailures(stage string) float64 {
	return auditWriteFailures.WithLabelValues(stage).Value()
}

func TestWriteFile_AuditStoreDown(t *testing.T) {
	dir := t.TempDir()
	c, _ := New(Config{ProjectRoot: dir}, downStore{logErr: errors.New("well unreachable")})
	path := filepath.Join(dir, "a.txt")
	before := auditFailures("log")
	if err := c.WriteFile(context.Background(), path, "x"); !errors.Is(err, ErrAuditUnavailable) {
		t.Fatalf("WriteFile = %v; want ErrAuditUnavailable", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("a write ran with no audit row")
	}
	if auditFailures("log") != before+1 {
		t.Error("the failed audit write was not counted")
	}
}

func TestMutations_AuditStoreDownFailClosed(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	existing := filepath.Join(dir, "p.txt")
	if err := os.WriteFile(existing, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	kb := newMockKanbanStore()
	c, _ := New(Config{ProjectRoot: dir}, downStore{logErr: errors.New("down")})
	c.WithKanbanStore(kb)
	checks := map[string]error{
		"patch":         c.PatchFile(ctx, existing, "old", "new"),
		"kanban_create": c.CreateKanbanTask(ctx, &KanbanTask{ID: "1", Title: "t"}),
		"kanban_update": c.UpdateKanbanTask(ctx, "1", map[string]any{"status": "done"}),
		"revert":        c.RevertAction(ctx, 1),
		"runbook_execute": func() error {
			_, err := c.RunbookExecute(ctx, "infra/x", true)
			return err
		}(),
		"model_switch": func() error {
			_, err := c.ModelSwap(ctx, "qwen-7b")
			return err
		}(),
	}
	for name, err := range checks {
		if !errors.Is(err, ErrAuditUnavailable) {
			t.Errorf("%s = %v; want ErrAuditUnavailable", name, err)
		}
	}
	if got, _ := os.ReadFile(existing); string(got) != "old" {
		t.Error("a patch ran with no audit row")
	}
	if len(kb.tasks) != 0 {
		t.Error("a kanban task was created with no audit row")
	}
}

func TestReadFile_AuditStoreDown(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "r.txt")
	if err := os.WriteFile(path, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _ := New(Config{ProjectRoot: dir}, downStore{logErr: errors.New("down")})
	before := auditFailures("log")
	got, err := c.ReadFile(context.Background(), path)
	if err != nil || got != "hi" {
		t.Fatalf("ReadFile = %q, %v; a read must survive an audit outage", got, err)
	}
	if auditFailures("log") != before+1 {
		t.Error("the failed audit write was not counted")
	}
}

func TestCompleteAction_FailureCounted(t *testing.T) {
	dir := t.TempDir()
	c, _ := New(Config{ProjectRoot: dir}, downStore{updateErr: errors.New("down")})
	before := auditFailures("complete")
	if err := c.WriteFile(context.Background(), filepath.Join(dir, "w.txt"), "x"); err != nil {
		t.Fatalf("WriteFile with a planned row recorded: %v", err)
	}
	if auditFailures("complete") != before+1 {
		t.Error("a failed completion update was not counted")
	}
}
