// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package lichtools

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

const confirmRoot = "/home/op/unheaded"

func newTestStore(t *testing.T) *PendingStore {
	t.Helper()
	s, err := NewPendingStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewPendingStore: %v", err)
	}
	return s
}

func TestPending_ParkThenApproveReleasesExactCall(t *testing.T) {
	s := newTestStore(t)
	args := json.RawMessage(`{"severity":"high","summary":"crash in parser"}`)

	tk, err := s.Park(ToolFileFinding, "high: crash in parser", args, confirmRoot)
	if err != nil {
		t.Fatalf("Park: %v", err)
	}
	if tk.Status != StatusPending {
		t.Fatalf("new ticket status = %q, want pending", tk.Status)
	}

	// Before approval, FindApproved must not release it.
	if _, ok, err := s.FindApproved(tk.CallHash); err != nil || ok {
		t.Fatalf("FindApproved before approve = (%v, %v), want (false, nil)", ok, err)
	}

	if _, err := s.Approve(tk.ID); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	// After approval, the SAME call (same canonical args → same hash) is released.
	got, ok, err := s.FindApproved(CallHash(ToolFileFinding, confirmRoot, args))
	if err != nil || !ok {
		t.Fatalf("FindApproved after approve = (%v, %v), want (true, nil)", ok, err)
	}
	if got.ID != tk.ID {
		t.Fatalf("released ticket id = %q, want %q", got.ID, tk.ID)
	}
}

func TestPending_DifferentInputHashDoesNotInheritApproval(t *testing.T) {
	s := newTestStore(t)
	approved := json.RawMessage(`{"severity":"high","summary":"A"}`)
	tk, _ := s.Park(ToolFileFinding, "A", approved, confirmRoot)
	if _, err := s.Approve(tk.ID); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	// A different argument set hashes differently and must NOT be released.
	other := json.RawMessage(`{"severity":"high","summary":"B"}`)
	if _, ok, err := s.FindApproved(CallHash(ToolFileFinding, confirmRoot, other)); err != nil || ok {
		t.Fatalf("different-args FindApproved = (%v, %v), want (false, nil)", ok, err)
	}
	// A different project root also voids the approval.
	if _, ok, _ := s.FindApproved(CallHash(ToolFileFinding, "/other/root", approved)); ok {
		t.Fatal("approval leaked across project roots")
	}
	// A different tool name also voids it.
	if _, ok, _ := s.FindApproved(CallHash(ToolRunHarness, confirmRoot, approved)); ok {
		t.Fatal("approval leaked across tools")
	}
}

func TestPending_CanonicalArgsHashIsOrderStable(t *testing.T) {
	a := json.RawMessage(`{"severity":"high","summary":"x","class":"mem"}`)
	b := json.RawMessage(`{"class":"mem","summary":"x","severity":"high"}`)
	if CallHash(ToolFileFinding, confirmRoot, a) != CallHash(ToolFileFinding, confirmRoot, b) {
		t.Fatal("key order changed the call hash; canonicalisation is not stable")
	}
}

func TestPending_ParkDedupsLiveTicket(t *testing.T) {
	s := newTestStore(t)
	args := json.RawMessage(`{"severity":"low","summary":"dup"}`)
	a, _ := s.Park(ToolFileFinding, "dup", args, confirmRoot)
	b, _ := s.Park(ToolFileFinding, "dup", args, confirmRoot)
	if a.ID != b.ID {
		t.Fatalf("re-parking the same call spawned a second ticket (%s vs %s)", a.ID, b.ID)
	}
	all, _ := s.List()
	if len(all) != 1 {
		t.Fatalf("store holds %d tickets, want 1", len(all))
	}
}

func TestPending_UsedTicketCannotReplay(t *testing.T) {
	s := newTestStore(t)
	args := json.RawMessage(`{"severity":"high","summary":"once"}`)
	tk, _ := s.Park(ToolFileFinding, "once", args, confirmRoot)
	if _, err := s.Approve(tk.ID); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if err := s.MarkUsed(tk.ID); err != nil {
		t.Fatalf("MarkUsed: %v", err)
	}
	if _, ok, _ := s.FindApproved(tk.CallHash); ok {
		t.Fatal("a used ticket was released again (replay)")
	}
}

func TestPending_ApproveUnknownAndNonPending(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Approve("0123456789abcdef"); !errors.Is(err, ErrNoSuchTicket) {
		t.Fatalf("Approve(unknown) err = %v, want ErrNoSuchTicket", err)
	}
	if _, err := s.Get("not-hex"); !errors.Is(err, ErrNoSuchTicket) {
		t.Fatalf("Get(malformed id) err = %v, want ErrNoSuchTicket", err)
	}
	tk, _ := s.Park(ToolFileFinding, "x", json.RawMessage(`{"severity":"low","summary":"x"}`), confirmRoot)
	if _, err := s.Approve(tk.ID); err != nil {
		t.Fatalf("first Approve: %v", err)
	}
	if _, err := s.Approve(tk.ID); !errors.Is(err, ErrNotPending) {
		t.Fatalf("second Approve err = %v, want ErrNotPending", err)
	}
}

func TestPending_ExpiredTicketIgnoredAndSwept(t *testing.T) {
	s := newTestStore(t)
	// Age the clock so the ticket is past its TTL.
	base := time.Now()
	s.now = func() time.Time { return base }
	args := json.RawMessage(`{"severity":"info","summary":"stale"}`)
	tk, _ := s.Park(ToolFileFinding, "stale", args, confirmRoot)
	s.now = func() time.Time { return base.Add(PendingTTL + time.Minute) }

	if _, err := s.Approve(tk.ID); !errors.Is(err, ErrNotPending) {
		t.Fatalf("Approve(expired) err = %v, want ErrNotPending", err)
	}
	// FindApproved sweeps expired tickets as a side effect; after that it is gone.
	if _, ok, _ := s.FindApproved(tk.CallHash); ok {
		t.Fatal("expired ticket released")
	}
	all, _ := s.List()
	if len(all) != 0 {
		t.Fatalf("expired ticket not swept: %d remain", len(all))
	}
}

func TestPending_SummaryIsControlCharSafe(t *testing.T) {
	s := newTestStore(t)
	tk, _ := s.Park(ToolFileFinding, "line1\nline2\x00\x1b[31m", json.RawMessage(`{"severity":"low","summary":"x"}`), confirmRoot)
	for _, r := range tk.Summary {
		if r < 0x20 || r == 0x7f {
			t.Fatalf("summary retained a control char %q", r)
		}
	}
}
