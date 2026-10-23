// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/pkg/claudebridge"
	"unheaded/pkg/lichtools"
	"unheaded/pkg/zhentools"
)

// fakeInnerGate stands in for the Champion gate: it refuses the mutating
// file_finding tool pending-confirmation and allows everything else.
type fakeInnerGate struct{}

func (fakeInnerGate) Execute(ctx context.Context, call claudebridge.ToolCall, p zhentools.Provider) (zhentools.Result, error) {
	if call.Name == lichtools.ToolFileFinding {
		return zhentools.Result{}, &claudebridge.RefusedError{Reason: "mutating tool, no justification", PendingConfirmation: true}
	}
	return p.Call(ctx, call.Name, call.Input)
}

// fakeProvider records whether file_finding actually ran.
type fakeProvider struct{ filed int }

func (f *fakeProvider) Tools(context.Context) ([]zhentools.Tool, error) { return nil, nil }
func (f *fakeProvider) Call(_ context.Context, name string, _ json.RawMessage) (zhentools.Result, error) {
	if name == lichtools.ToolFileFinding {
		f.filed++
		return zhentools.Result{Text: "filed FND-999"}, nil
	}
	return zhentools.Result{Text: "ok"}, nil
}

func newGateForTest(t *testing.T) (*confirmGate, *lichtools.PendingStore, *fakeProvider) {
	t.Helper()
	store, err := lichtools.NewPendingStore(t.TempDir())
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	g := newConfirmGate(fakeInnerGate{}, store, confirmRootForTest, nil)
	return g, store, &fakeProvider{}
}

const confirmRootForTest = "/home/op/unheaded"

func fileFindingCall(args string) claudebridge.ToolCall {
	return claudebridge.ToolCall{Name: lichtools.ToolFileFinding, Input: json.RawMessage(args), Provider: "lich-mcp"}
}

func TestConfirmGate_ParksThenApprovesExactlyOneCall(t *testing.T) {
	g, store, prov := newGateForTest(t)
	ctx := context.Background()
	call := fileFindingCall(`{"severity":"high","summary":"parser crash"}`)

	// First attempt: refused pending, a ticket is parked.
	_, err := g.Execute(ctx, call, prov)
	var ref *claudebridge.RefusedError
	if !errors.As(err, &ref) || !ref.PendingConfirmation {
		t.Fatalf("first Execute err = %v, want pending RefusedError", err)
	}
	if prov.filed != 0 {
		t.Fatalf("file_finding ran before approval (%d)", prov.filed)
	}
	tickets, _ := store.List()
	if len(tickets) != 1 {
		t.Fatalf("parked %d tickets, want 1", len(tickets))
	}
	if !strings.Contains(ref.Reason, tickets[0].ID) {
		t.Fatalf("refusal reason does not name the ticket id %q: %q", tickets[0].ID, ref.Reason)
	}

	// Re-issuing before approval does not run it and does not spawn a second ticket.
	if _, err := g.Execute(ctx, call, prov); !errors.As(err, &ref) || !ref.PendingConfirmation {
		t.Fatalf("second Execute (unapproved) err = %v, want pending", err)
	}
	if again, _ := store.List(); len(again) != 1 {
		t.Fatalf("re-issue spawned a duplicate ticket: %d", len(again))
	}

	// Operator approves out of band.
	if _, err := store.Approve(tickets[0].ID); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	// Re-issuing the identical call now runs exactly once.
	res, err := g.Execute(ctx, call, prov)
	if err != nil {
		t.Fatalf("Execute after approve: %v", err)
	}
	if res.Text != "filed FND-999" || prov.filed != 1 {
		t.Fatalf("approved call result=%q filed=%d, want filed once", res.Text, prov.filed)
	}

	// A further re-issue is refused again (the approval was single-use).
	if _, err := g.Execute(ctx, call, prov); !errors.As(err, &ref) || !ref.PendingConfirmation {
		t.Fatalf("post-redeem Execute err = %v, want pending (no replay)", err)
	}
	if prov.filed != 1 {
		t.Fatalf("approval was replayed: filed=%d", prov.filed)
	}
}

func TestConfirmGate_DifferentCallDoesNotInheritApproval(t *testing.T) {
	g, store, prov := newGateForTest(t)
	ctx := context.Background()

	approved := fileFindingCall(`{"severity":"high","summary":"A"}`)
	_, _ = g.Execute(ctx, approved, prov)
	tickets, _ := store.List()
	if _, err := store.Approve(tickets[0].ID); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	// A different finding must not ride the approval.
	other := fileFindingCall(`{"severity":"high","summary":"B"}`)
	_, err := g.Execute(ctx, other, prov)
	var ref *claudebridge.RefusedError
	if !errors.As(err, &ref) || !ref.PendingConfirmation {
		t.Fatalf("different call err = %v, want pending", err)
	}
	if prov.filed != 0 {
		t.Fatalf("a different finding was filed on another's approval (%d)", prov.filed)
	}
}

func TestConfirmGate_ReadOnlyToolUntouched(t *testing.T) {
	g, _, prov := newGateForTest(t)
	res, err := g.Execute(context.Background(),
		claudebridge.ToolCall{Name: lichtools.ToolListCampaigns, Input: json.RawMessage(`{}`), Provider: "lich-mcp"}, prov)
	if err != nil || res.Text != "ok" {
		t.Fatalf("read-only tool through confirmGate = (%q, %v), want ok", res.Text, err)
	}
}

func TestConfirmCLI_ListApproveShow(t *testing.T) {
	dir := t.TempDir()
	pending := filepath.Join(dir, "pending")
	cfgPath := filepath.Join(dir, "lich-mcp.yaml")
	if err := os.WriteFile(cfgPath, []byte("pending_dir: "+pending+"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Park a ticket directly in the store the CLI will read.
	store, err := lichtools.NewPendingStore(pending)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	tk, err := store.Park(lichtools.ToolFileFinding, "high: x", json.RawMessage(`{"severity":"high","summary":"x"}`), dir)
	if err != nil {
		t.Fatalf("Park: %v", err)
	}

	// list shows it as pending.
	var out, errb bytes.Buffer
	if code := runConfirm([]string{"-config", cfgPath, "-project-root", dir, "list"}, &out, &errb); code != 0 {
		t.Fatalf("list exit %d, stderr=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), tk.ID) || !strings.Contains(out.String(), "pending") {
		t.Fatalf("list output missing ticket: %q", out.String())
	}

	// approve flips it.
	out.Reset()
	errb.Reset()
	if code := runConfirm([]string{"-config", cfgPath, "-project-root", dir, "approve", tk.ID}, &out, &errb); code != 0 {
		t.Fatalf("approve exit %d, stderr=%s", code, errb.String())
	}
	got, _ := store.Get(tk.ID)
	if got.Status != lichtools.StatusApproved {
		t.Fatalf("ticket status after approve = %q, want approved", got.Status)
	}

	// approve again refuses (not pending).
	out.Reset()
	errb.Reset()
	if code := runConfirm([]string{"-config", cfgPath, "-project-root", dir, "approve", tk.ID}, &out, &errb); code == 0 {
		t.Fatalf("second approve should fail, got exit 0")
	}

	// approve an unknown id errors.
	out.Reset()
	errb.Reset()
	if code := runConfirm([]string{"-config", cfgPath, "-project-root", dir, "approve", "0123456789abcdef"}, &out, &errb); code == 0 {
		t.Fatalf("approve unknown id should fail, got exit 0")
	}
}
