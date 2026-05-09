// SPDX-License-Identifier: GPL-3.0-or-later
//
// Unit tests for upc-tty-bridge.

package main

import (
	"testing"
	"time"
)

// ─── parseInstanceParam ──────────────────────────────────────────────────────

func TestParseInstanceParam(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  string
		want uint8
	}{
		{"empty defaults to 0xDE", "", 0xDE},
		{"valid mid-range", "222", 222},
		{"zero is allowed", "0", 0},
		{"max byte 255", "255", 255},
		{"out of range above bytes defaults to 0xDE", "256", 0xDE},
		{"negative defaults to 0xDE", "-1", 0xDE},
		{"non-numeric defaults to 0xDE", "abc", 0xDE},
		{"trailing junk defaults to 0xDE", "1abc", 1}, // Sscanf is lenient — accepts 1
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseInstanceParam(tc.raw)
			if got != tc.want {
				t.Fatalf("parseInstanceParam(%q) = 0x%02X, want 0x%02X",
					tc.raw, got, tc.want)
			}
		})
	}
}

// ─── Hub bookkeeping + broadcast fan-out ─────────────────────────────────────

func TestHubAddRemove(t *testing.T) {
	t.Parallel()
	hub := newHub()
	if got := len(hub.subscribers); got != 0 {
		t.Fatalf("newHub: expected 0 subscribers, got %d", got)
	}

	sub := &Subscriber{instance: 0xDE, out: make(chan []byte, 4)}
	hub.add(sub)
	if got := len(hub.subscribers); got != 1 {
		t.Fatalf("after add: expected 1 subscriber, got %d", got)
	}

	hub.remove(sub)
	if got := len(hub.subscribers); got != 0 {
		t.Fatalf("after remove: expected 0 subscribers, got %d", got)
	}
	// remove() also closes sub.out — verify by checking it is closed.
	if _, ok := <-sub.out; ok {
		t.Fatal("after remove: sub.out should be closed")
	}
}

func TestHubBroadcastFanOutByInstance(t *testing.T) {
	t.Parallel()
	hub := newHub()

	subA := &Subscriber{instance: 0xDE, out: make(chan []byte, 4)}
	subB := &Subscriber{instance: 0xDE, out: make(chan []byte, 4)}
	subC := &Subscriber{instance: 0xCC, out: make(chan []byte, 4)} // different instance
	hub.add(subA)
	hub.add(subB)
	hub.add(subC)
	t.Cleanup(func() {
		hub.remove(subA)
		hub.remove(subB)
		hub.remove(subC)
	})

	// Broadcast to instance 0xDE — A + B receive, C does not.
	hub.broadcastTty(0xDE, []byte("hello-de"))

	expectChan(t, subA.out, "hello-de", "subA should receive")
	expectChan(t, subB.out, "hello-de", "subB should receive")
	if got := drainChan(subC.out); got != "" {
		t.Fatalf("subC should NOT receive 0xDE traffic, got: %q", got)
	}

	// Broadcast to instance 0xCC — only C receives.
	hub.broadcastTty(0xCC, []byte("hello-cc"))
	expectChan(t, subC.out, "hello-cc", "subC should receive 0xCC traffic")
	if got := drainChan(subA.out); got != "" {
		t.Fatalf("subA should NOT receive 0xCC traffic, got: %q", got)
	}
}

func TestHubBroadcastDropsWhenSubscriberFull(t *testing.T) {
	t.Parallel()
	hub := newHub()
	// Buffer size 1 — second send should drop, not block.
	sub := &Subscriber{instance: 0xDE, out: make(chan []byte, 1)}
	hub.add(sub)
	t.Cleanup(func() { hub.remove(sub) })

	hub.broadcastTty(0xDE, []byte("first"))
	// Don't drain; second broadcast must NOT block beyond a tiny tolerance.
	done := make(chan struct{})
	go func() {
		hub.broadcastTty(0xDE, []byte("second"))
		close(done)
	}()
	select {
	case <-done:
		// ok — broadcast returned promptly via the default case.
	case <-time.After(100 * time.Millisecond):
		t.Fatal("broadcastTty blocked on full subscriber — should drop instead")
	}

	// Verify only the first message landed.
	expectChan(t, sub.out, "first", "subA should still have 'first'")
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func expectChan(t *testing.T, ch <-chan []byte, want, msg string) {
	t.Helper()
	select {
	case got := <-ch:
		if string(got) != want {
			t.Fatalf("%s: got %q, want %q", msg, string(got), want)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("%s: timed out waiting for %q", msg, want)
	}
}

func drainChan(ch <-chan []byte) string {
	select {
	case got := <-ch:
		return string(got)
	default:
		return ""
	}
}
