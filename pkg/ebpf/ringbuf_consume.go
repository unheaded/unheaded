// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package ebpf

import (
	"sync/atomic"
	"unsafe"
)

// consumeRingbuf drains committed records from a BPF ring buffer.
//
// cons and prod are the consumer and producer positions (the first words of
// the ring's two metadata pages), data is the data area, which the kernel
// maps twice back to back so a record that wraps reads contiguously: it is
// 2*ringSize long. Each record is an 8-byte header (length with the busy
// and discard bits, then the kernel's page offset) and the payload, padded
// to 8 bytes. emit receives a copy of each payload; returning false stops
// early (the record still counts as consumed). It returns how many records
// it emitted.
//
// Positions are read with acquire and written with release semantics, as
// the kernel's side of the protocol expects.
func consumeRingbuf(cons, prod *uint64, data []byte, ringSize int, emit func([]byte) bool) int {
	n := 0
	for {
		c := atomic.LoadUint64(cons)
		p := atomic.LoadUint64(prod)
		if c >= p {
			return n
		}
		off := int(c % uint64(ringSize)) // #nosec G115 -- bounded by the modulo against ringSize
		if off+BPF_RINGBUF_HDR_SZ > len(data) {
			return n
		}
		hdr := atomic.LoadUint32((*uint32)(unsafe.Pointer(&data[off]))) // #nosec G103 -- aligned header word in the kernel-mapped ring; records are 8-byte aligned
		if hdr&BPF_RINGBUF_BUSY_BIT != 0 {
			return n // reserved, not yet committed
		}
		dataLen := int(hdr &^ uint32(BPF_RINGBUF_BUSY_BIT|BPF_RINGBUF_DISCARD_BIT))
		next := c + uint64(roundUp(BPF_RINGBUF_HDR_SZ+dataLen, 8)) // #nosec G115 -- small non-negative length
		if hdr&BPF_RINGBUF_DISCARD_BIT == 0 {
			start := off + BPF_RINGBUF_HDR_SZ
			if start+dataLen > len(data) {
				return n // malformed length: leave it rather than read past the mapping
			}
			rec := make([]byte, dataLen)
			copy(rec, data[start:start+dataLen])
			n++
			atomic.StoreUint64(cons, next)
			if !emit(rec) {
				return n
			}
			continue
		}
		atomic.StoreUint64(cons, next)
	}
}
