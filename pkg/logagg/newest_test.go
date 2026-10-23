// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package logagg

import (
	"fmt"
	"testing"
)

func fill(rb *RingBuffer, n int) {
	for i := 0; i < n; i++ {
		rb.Push(LogEntry{Service: "svc", Level: "info", Message: fmt.Sprintf("m%d", i)})
	}
}

func msgs(es []LogEntry) string {
	out := ""
	for _, e := range es {
		out += e.Message + " "
	}
	return out
}

// "The last 3" used to be the first 3 the buffer still held.
func TestQueryNewest(t *testing.T) {
	rb := NewRingBuffer(5)
	fill(rb, 8) // holds m3..m7
	if got := msgs(rb.Query(LogQuery{Limit: 3, Newest: true})); got != "m5 m6 m7 " {
		t.Errorf("newest 3 = %q, want m5 m6 m7", got)
	}
	if got := msgs(rb.Query(LogQuery{Limit: 2, Offset: 1, Newest: true})); got != "m5 m6 " {
		t.Errorf("newest 2 skipping 1 = %q, want m5 m6", got)
	}
	if got := msgs(rb.Query(LogQuery{Limit: 3})); got != "m3 m4 m5 " {
		t.Errorf("default (oldest first) changed: %q", got)
	}
	if got := rb.Query(LogQuery{Limit: 3, Newest: true, Service: "nobody"}); len(got) != 0 {
		t.Errorf("filtered newest = %v, want empty", got)
	}
}

// The dashboard's live tail polled Len, which stops growing once the ring
// is full, so it went silent after 10,000 entries. Since uses a sequence.
func TestSinceKeepsWorkingWhenFull(t *testing.T) {
	rb := NewRingBuffer(4)
	fill(rb, 4)
	seq := rb.Seq()
	if rb.Len() != 4 || seq != 4 {
		t.Fatalf("len %d seq %d", rb.Len(), seq)
	}
	fill(rb, 2) // m0 m1 again, buffer still Len 4
	got, next := rb.Since(seq)
	if msgs(got) != "m0 m1 " || next != 6 {
		t.Errorf("Since(4) = %q, %d; want the 2 new entries, 6", msgs(got), next)
	}
	if got, _ := rb.Since(next); len(got) != 0 {
		t.Errorf("nothing new, got %q", msgs(got))
	}
	fill(rb, 10) // more than capacity since the last look
	got, next = rb.Since(next)
	if len(got) != 4 || next != 16 {
		t.Errorf("after overflow: %d entries, seq %d; want the 4 still held, 16", len(got), next)
	}
	if got, _ := NewRingBuffer(3).Since(0); len(got) != 0 {
		t.Errorf("empty buffer returned %v", got)
	}
}
