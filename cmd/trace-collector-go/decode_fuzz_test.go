// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// LICH-005, userspace half: these decoders take bytes the kernel wrote into
// BPF maps and ring buffers. Any input must yield a value or an error, never
// a panic, and where an Encode exists, decode(encode(decode(b))) == decode(b).
//
//	go test ./cmd/trace-collector-go -run '^$' -fuzz FuzzDecodeFlowEvent -fuzztime 30s

package main

import (
	"reflect"
	"testing"
)

func seedSizes(f *testing.F, sizes ...int) {
	f.Add([]byte{})
	for _, n := range sizes {
		for _, fill := range []byte{0x00, 0xff, 0x5a} {
			b := make([]byte, n)
			for i := range b {
				b[i] = fill
			}
			f.Add(b)
			f.Add(b[:n-1])
		}
	}
}

func FuzzDecodeTraceEntry(f *testing.F) {
	seedSizes(f, TraceEntrySize)
	f.Fuzz(func(t *testing.T, b []byte) {
		v, err := DecodeTraceEntry(b)
		if err != nil {
			return
		}
		enc := v.Encode()
		v2, err := DecodeTraceEntry(enc[:])
		if err != nil || !reflect.DeepEqual(v, v2) {
			t.Fatalf("round trip: %+v -> %+v (%v)", v, v2, err)
		}
	})
}

func FuzzDecodeStatsEntry(f *testing.F) {
	seedSizes(f, StatsEntrySize)
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = DecodeStatsEntry(b) })
}

func FuzzDecodeFlowKey(f *testing.F) {
	seedSizes(f, FlowKeySize)
	f.Fuzz(func(t *testing.T, b []byte) {
		v, err := DecodeFlowKey(b)
		if err != nil {
			return
		}
		enc := v.Encode()
		v2, err := DecodeFlowKey(enc[:])
		if err != nil || !reflect.DeepEqual(v, v2) {
			t.Fatalf("round trip: %+v -> %+v (%v)", v, v2, err)
		}
	})
}

func FuzzDecodeFlowState(f *testing.F) {
	seedSizes(f, FlowStateSize)
	f.Fuzz(func(t *testing.T, b []byte) {
		v, err := DecodeFlowState(b)
		if err != nil {
			return
		}
		enc := v.Encode()
		v2, err := DecodeFlowState(enc[:])
		if err != nil || !reflect.DeepEqual(v, v2) {
			t.Fatalf("round trip: %+v -> %+v (%v)", v, v2, err)
		}
	})
}

func FuzzDecodeFlowEvent(f *testing.F) {
	seedSizes(f, FlowEventSize)
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = DecodeFlowEvent(b) })
}

func FuzzDecodeLatencyKey(f *testing.F) {
	seedSizes(f, LatencyKeySize)
	f.Fuzz(func(t *testing.T, b []byte) {
		v, err := DecodeLatencyKey(b)
		if err != nil {
			return
		}
		enc := v.Encode()
		v2, err := DecodeLatencyKey(enc[:])
		if err != nil || !reflect.DeepEqual(v, v2) {
			t.Fatalf("round trip: %+v -> %+v (%v)", v, v2, err)
		}
	})
}

func FuzzDecodeLatencyEntry(f *testing.F) {
	seedSizes(f, LatencyEntrySize)
	f.Fuzz(func(t *testing.T, b []byte) {
		v, err := DecodeLatencyEntry(b)
		if err != nil {
			return
		}
		enc := v.Encode()
		v2, err := DecodeLatencyEntry(enc[:])
		if err != nil || !reflect.DeepEqual(v, v2) {
			t.Fatalf("round trip: %+v -> %+v (%v)", v, v2, err)
		}
	})
}
