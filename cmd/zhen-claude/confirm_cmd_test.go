// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/champion"
	"unheaded/pkg/claudebridge"
)

// memStore is an in-memory confirmStore with the table's one-way rules.
type memStore struct {
	mu   sync.Mutex
	rows map[string]*claudebridge.QueuedCall
}

func (m *memStore) List(context.Context, int) ([]claudebridge.QueuedCall, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []claudebridge.QueuedCall
	for _, r := range m.rows {
		if r.Status == "pending" {
			out = append(out, *r)
		}
	}
	return out, nil
}

func (m *memStore) Get(_ context.Context, id string) (claudebridge.QueuedCall, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[id]
	if !ok {
		return claudebridge.QueuedCall{}, errors.New("no such confirmation")
	}
	return *r, nil
}

func (m *memStore) Decide(_ context.Context, id, status, by string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[id]
	if !ok || r.Status != "pending" {
		return claudebridge.ErrNotPending
	}
	if status == "approved" && !time.Now().Before(r.ExpiresAt) {
		return errors.New("expired")
	}
	r.Status, r.DecidedBy = status, by
	return nil
}

func (m *memStore) Finish(_ context.Context, id, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[id]
	if !ok || r.Status != "approved" {
		return claudebridge.ErrNotPending
	}
	r.Status = status
	return nil
}

const testID = "0123456789abcdef"

// queued builds a pending row the way QueueConfirmer would.
func queued(t *testing.T, root, file, content string, ttl time.Duration) *claudebridge.QueuedCall {
	t.Helper()
	c, err := champion.New(champion.Config{ProjectRoot: root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err := c.PendingCallFor(champion.ToolCall{Name: "write_file", Args: map[string]any{"path": filepath.Join(root, file), "content": content}, EmittedBy: "claude-api/m/c"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(p.Args)
	return &claudebridge.QueuedCall{ID: testID, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(ttl), SourceService: "zhen-claude",
		EmittedBy: p.EmittedBy, ProjectRoot: root, Tool: p.Tool, Call: args, CallSHA256: p.SHA256, Summary: "write_file " + file, Status: "pending"}
}

func withStore(t *testing.T, rows ...*claudebridge.QueuedCall) *memStore {
	t.Helper()
	m := &memStore{rows: map[string]*claudebridge.QueuedCall{}}
	for _, r := range rows {
		m.rows[r.ID] = r
	}
	prev := openConfirmStore
	openConfirmStore = func(context.Context) (confirmStore, func(), error) { return m, func() {}, nil }
	prevBy := confirmedBy
	confirmedBy = func() string { return "stevie@test" }
	t.Cleanup(func() { openConfirmStore, confirmedBy = prev, prevBy })
	return m
}

func confirmRun(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(append([]string{"confirm"}, args...), strings.NewReader(""), &out, &errb)
	return code, out.String(), errb.String()
}

func TestConfirmCmd_ApproveRunsOnceAndOnlyOnce(t *testing.T) {
	root := t.TempDir()
	m := withStore(t, queued(t, root, "a.txt", "hello\n", time.Hour))
	code, out, errs := confirmRun("-allowed-roots", root, "approve", testID)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "a.txt")); string(got) != "hello\n" {
		t.Fatalf("file = %q", got)
	}
	if m.rows[testID].Status != "executed" || m.rows[testID].DecidedBy != "stevie@test" {
		t.Errorf("row = %+v", m.rows[testID])
	}
	if !strings.Contains(out, "+hello") {
		t.Errorf("approve did not show the diff it ran: %s", out)
	}
	if code, _, errs := confirmRun("-allowed-roots", root, "approve", testID); code != 1 || !strings.Contains(errs, "not pending") {
		t.Errorf("second approve: exit %d, %s", code, errs)
	}
}

func TestConfirmCmd_ExpiredRefused(t *testing.T) {
	root := t.TempDir()
	m := withStore(t, queued(t, root, "a.txt", "x", -time.Minute))
	if code, _, errs := confirmRun("-allowed-roots", root, "approve", testID); code != 1 || !strings.Contains(errs, "expired") {
		t.Errorf("exit %d, %s", code, errs)
	}
	if m.rows[testID].Status != "expired" {
		t.Errorf("status = %s", m.rows[testID].Status)
	}
	if _, err := os.Stat(filepath.Join(root, "a.txt")); !os.IsNotExist(err) {
		t.Error("an expired call ran")
	}
}

func TestConfirmCmd_RootNotAllowlisted(t *testing.T) {
	root := t.TempDir()
	withStore(t, queued(t, root, "a.txt", "x", time.Hour))
	if code, _, errs := confirmRun("-allowed-roots", t.TempDir(), "approve", testID); code != 1 || !strings.Contains(errs, "allowed-roots") {
		t.Errorf("exit %d, %s", code, errs)
	}
	if _, err := os.Stat(filepath.Join(root, "a.txt")); !os.IsNotExist(err) {
		t.Error("a call for an unlisted root ran")
	}
}

func TestConfirmCmd_TamperedCallRefused(t *testing.T) {
	root := t.TempDir()
	r := queued(t, root, "a.txt", "benign", time.Hour)
	r.Call = json.RawMessage(`{"path":"` + filepath.Join(root, "a.txt") + `","content":"evil"}`)
	m := withStore(t, r)
	if code, _, errs := confirmRun("-allowed-roots", root, "approve", testID); code != 1 || !strings.Contains(errs, "hash") {
		t.Errorf("exit %d, %s", code, errs)
	}
	if m.rows[testID].Status != "pending" {
		t.Errorf("a tampered row was decided: %s", m.rows[testID].Status)
	}
	if _, err := os.Stat(filepath.Join(root, "a.txt")); !os.IsNotExist(err) {
		t.Error("a tampered call ran")
	}
}

func TestConfirmCmd_RootSwapRefused(t *testing.T) {
	// The row's call was hashed for root A; someone rewrites project_root
	// to B (also allowlisted). The hash binds the root, so it refuses.
	a, b := t.TempDir(), t.TempDir()
	r := queued(t, a, "a.txt", "x", time.Hour)
	r.ProjectRoot = b
	withStore(t, r)
	if code, _, errs := confirmRun("-allowed-roots", a+","+b, "approve", testID); code != 1 || !strings.Contains(errs, "hash") {
		t.Errorf("exit %d, %s", code, errs)
	}
}

func TestConfirmCmd_DenyListShow(t *testing.T) {
	root := t.TempDir()
	m := withStore(t, queued(t, root, "a.txt", "line\n", time.Hour))
	code, out, _ := confirmRun("list")
	if code != 0 || !strings.Contains(out, testID) {
		t.Errorf("list: %d %s", code, out)
	}
	code, out, _ = confirmRun("show", testID)
	if code != 0 || !strings.Contains(out, "+line") || !strings.Contains(out, "sha256:") {
		t.Errorf("show: %d %s", code, out)
	}
	if code, _, _ := confirmRun("deny", testID); code != 0 || m.rows[testID].Status != "denied" {
		t.Errorf("deny: %d %s", code, m.rows[testID].Status)
	}
	if code, _, _ := confirmRun("-allowed-roots", root, "approve", testID); code != 1 {
		t.Error("a denied call was approved")
	}
}

func TestConfirmCmd_Usage(t *testing.T) {
	withStore(t)
	for _, args := range [][]string{{}, {"approve"}, {"approve", "XYZ"}, {"frobnicate"}, {"list", "extra"}} {
		if code, _, _ := confirmRun(args...); code != 2 {
			t.Errorf("%v: exit %d", args, code)
		}
	}
}

func TestConfirmFlag_Validation(t *testing.T) {
	noCredentials(t)
	t.Setenv(envAppZhenDSN, "")
	var out, errb bytes.Buffer
	if code := run(append(baseArgs(t), "-confirm", "queue", "-p", "x"), strings.NewReader(""), &out, &errb); code != 2 || !strings.Contains(errb.String(), envAppZhenDSN) {
		t.Errorf("queue without DSN: exit %d %s", code, errb.String())
	}
	errb.Reset()
	if code := run(append(baseArgs(t), "-confirm", "yolo", "-p", "x"), strings.NewReader(""), &out, &errb); code != 2 {
		t.Errorf("bad mode: exit %d", code)
	}
}

func TestConfirmTTY_NoTerminalExits2(t *testing.T) {
	noCredentials(t)
	prev := openTTY
	openTTY = func() (io.ReadWriteCloser, error) { return nil, errors.New("no such device or address") }
	t.Cleanup(func() { openTTY = prev })
	var out, errb bytes.Buffer
	if code := run(append(baseArgs(t), "-confirm", "tty", "-p", "x"), strings.NewReader(""), &out, &errb); code != 2 || !strings.Contains(errb.String(), "controlling terminal") {
		t.Errorf("exit %d: %s", code, errb.String())
	}
}

func TestConfirmDefault_IsDeny(t *testing.T) {
	o, err := parseFlags(baseArgs(t), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if o.confirm != "deny" {
		t.Errorf("default -confirm = %q", o.confirm)
	}
}
