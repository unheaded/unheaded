// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// RT-03 (S-CHAMPION-HARDENING Steps 42-43): a confirmation is bound to the
// principal that requested the call. agent:* never confirms; ui:/tty:
// confirm only their own requests; queue:stevie (the Well queue, Stevie's
// own login) confirms any.

func boundCall(c *Champion, name, requester string) ToolCall {
	tc := writeCall(c, name, "x\n")
	tc.EmittedBy = "agent:zhen-7b"
	tc.Principal = requester
	return tc
}

func TestConfirmerAllowed(t *testing.T) {
	cases := []struct {
		requester, by string
		want          bool
	}{
		{"ui:stevie", "ui:stevie", true},
		{"tty:stevie@west", "tty:stevie@west", true},
		{"ui:stevie", "queue:stevie", true},
		{"", "queue:stevie", true},
		{"ui:stevie", "ui:mallory", false},
		{"ui:stevie", "tty:stevie", false},
		{"ui:stevie", "agent:zhen-7b", false},
		{"agent:zhen-7b", "agent:zhen-7b", false},
		{"ui:stevie", "", false},
		{"", "", false},
		{"", "ui:stevie", false},
		{"ui:", "ui:", false},
		{"ui:stevie", "stevie", false},
		{"stevie", "stevie", false},
		{"ui:stevie", "queue:mallory", false},
		{"ui:stevie", "Queue:stevie", false},
		{"ui:stevie ", "ui:stevie ", false},
		{"ui:ste\x00vie", "ui:ste\x00vie", false},
	}
	for _, tc := range cases {
		if got := confirmerAllowed(tc.requester, tc.by); got != tc.want {
			t.Errorf("confirmerAllowed(%q, %q) = %v; want %v", tc.requester, tc.by, got, tc.want)
		}
	}
}

func TestConfirmCall_BoundToRequester(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestChampion(t)
	p, err := c.PendingCallFor(boundCall(c, "a.txt", "ui:stevie"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if p.Requester != "ui:stevie" {
		t.Fatalf("requester = %q", p.Requester)
	}
	for _, by := range []string{"ui:mallory", "agent:zhen-7b", "stevie", "", "tty:stevie"} {
		if _, err := c.ConfirmCall(ctx, p, p.SHA256, by); err == nil {
			t.Errorf("confirmer %q ran a call requested by ui:stevie", by)
		}
	}
	// Re-binding the requester changes the hash: the approval does not carry over.
	re := p
	re.Requester = "ui:mallory"
	if _, err := c.ConfirmCall(ctx, re, p.SHA256, "ui:mallory"); err == nil {
		t.Error("a call re-bound to another requester ran under the original approval")
	}
	if _, err := os.Stat(filepath.Join(c.config.ProjectRoot, "a.txt")); !os.IsNotExist(err) {
		t.Fatal("the file was written by a refused confirmer")
	}
	if _, err := c.ConfirmCall(ctx, p, p.SHA256, "ui:stevie"); err != nil {
		t.Fatalf("the requester's own confirmation: %v", err)
	}
	q, _ := c.PendingCallFor(boundCall(c, "b.txt", "ui:stevie"), time.Minute)
	if _, err := c.ConfirmCall(ctx, q, q.SHA256, "queue:stevie"); err != nil {
		t.Fatalf("queue:stevie: %v", err)
	}
	// No requester: only the queue may confirm.
	n, _ := c.PendingCallFor(boundCall(c, "c.txt", ""), time.Minute)
	if _, err := c.ConfirmCall(ctx, n, n.SHA256, "tty:stevie@west"); err == nil {
		t.Error("a requester-less call ran under a tty confirmer")
	}
	if _, err := c.ConfirmCall(ctx, n, n.SHA256, "queue:stevie"); err != nil {
		t.Errorf("queue:stevie on a requester-less call: %v", err)
	}
}

func TestCanonicalCall_RequesterBound(t *testing.T) {
	_, plain, err := CanonicalCall("write_file", map[string]any{"path": "a"}, "/r")
	if err != nil {
		t.Fatal(err)
	}
	_, empty, _ := CanonicalCallFor("write_file", map[string]any{"path": "a"}, "/r", "")
	_, a, _ := CanonicalCallFor("write_file", map[string]any{"path": "a"}, "/r", "ui:a")
	_, b, _ := CanonicalCallFor("write_file", map[string]any{"path": "a"}, "/r", "ui:b")
	if plain != empty {
		t.Error("an empty requester must hash like the requester-less call (queued rows predate the field)")
	}
	if a == b || a == plain {
		t.Error("the requester does not change the hash")
	}
}

func TestConfirmPendingToolCall_BoundToRequester(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestChampion(t)
	tc := untrustedWrite(t, c) // Principal ui:stevie
	tok, err := c.IssuePendingConfirmation(tc)
	if err != nil {
		t.Fatal(err)
	}
	for _, by := range []string{"ui:mallory", "agent:zhen-agent", "", "stevie"} {
		_, err := c.ConfirmPendingToolCall(ctx, tok, by)
		if err == nil {
			t.Fatalf("confirmer %q redeemed ui:stevie's token", by)
		}
		if !strings.Contains(err.Error(), "may not confirm") {
			t.Errorf("confirmer %q: error %v; want a principal refusal", by, err)
		}
	}
	// A refused confirmer does not burn the requester's token.
	if _, err := c.ConfirmPendingToolCall(ctx, tok, "ui:stevie"); err != nil {
		t.Fatalf("the requester's own confirmation after refusals: %v", err)
	}
}
