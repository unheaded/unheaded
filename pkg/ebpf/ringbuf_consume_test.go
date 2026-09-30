// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package ebpf

import (
	"encoding/binary"
	"fmt"
	"testing"
)

// fakeRing builds a ring's data area (2*size, the second half mirroring the
// first as the kernel's double mapping does) from records written at pos.
type fakeRing struct {
	size int
	data []byte
	prod uint64
}

func newFakeRing(size int) *fakeRing { return &fakeRing{size: size, data: make([]byte, 2*size)} }

func (r *fakeRing) put(payload string, flags uint32) {
	off := int(r.prod % uint64(r.size))
	var hdr [8]byte
	binary.LittleEndian.PutUint32(hdr[:], uint32(len(payload))|flags)
	rec := append(hdr[:], payload...)
	for i, b := range rec {
		r.data[(off+i)%r.size] = b
		r.data[r.size+(off+i)%r.size] = b
	}
	r.prod += uint64(roundUp(8+len(payload), 8))
}

func drain(r *fakeRing, cons *uint64) []string {
	var got []string
	consumeRingbuf(cons, &r.prod, r.data, r.size, func(b []byte) bool { got = append(got, string(b)); return true })
	return got
}

func TestConsumeRingbuf(t *testing.T) {
	r := newFakeRing(64)
	var cons uint64
	r.put("alpha", 0)
	r.put("skip", BPF_RINGBUF_DISCARD_BIT)
	r.put("bravo", 0)
	if got := drain(r, &cons); fmt.Sprint(got) != "[alpha bravo]" || cons != r.prod {
		t.Fatalf("got %v cons %d prod %d, want [alpha bravo] and cons == prod", got, cons, r.prod)
	}
	// A reserved (busy) record stops the drain; nothing after it is read.
	r.put("pending", BPF_RINGBUF_BUSY_BIT)
	busyAt := cons
	r.put("later", 0)
	if got := drain(r, &cons); len(got) != 0 || cons != busyAt {
		t.Fatalf("past a busy record: got %v cons %d, want nothing and cons %d", got, cons, busyAt)
	}
	// Once committed, both come through.
	binary.LittleEndian.PutUint32(r.data[int(busyAt)%r.size:], uint32(len("pending")))
	binary.LittleEndian.PutUint32(r.data[r.size+int(busyAt)%r.size:], uint32(len("pending")))
	if got := drain(r, &cons); fmt.Sprint(got) != "[pending later]" {
		t.Fatalf("after commit: %v", got)
	}
}

// A record that crosses the end of the ring reads contiguously through the
// second mapping.
func TestConsumeRingbuf_Wraps(t *testing.T) {
	r := newFakeRing(32)
	var cons uint64
	r.put("0123456789", 0) // 24 bytes: 0..24
	drain(r, &cons)
	r.put("wrapping-rec", 0) // starts at 24, crosses 32
	if got := drain(r, &cons); fmt.Sprint(got) != "[wrapping-rec]" {
		t.Fatalf("wrapped record = %v", got)
	}
}

// A length that would read past the mapping is left unconsumed.
func TestConsumeRingbuf_MalformedLength(t *testing.T) {
	r := newFakeRing(32)
	var cons uint64
	binary.LittleEndian.PutUint32(r.data[0:], 1000)
	r.prod = 1008
	if got := drain(r, &cons); len(got) != 0 || cons != 0 {
		t.Fatalf("malformed: got %v cons %d", got, cons)
	}
}
