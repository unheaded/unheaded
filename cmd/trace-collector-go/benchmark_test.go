// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
	"os"
	"testing"
	"time"

	ciliumebpf "github.com/cilium/ebpf"
)

// ── Benchmark: TraceEntry decode ────────────────────────────────────────────

// ── Benchmark: TraceEntry encode ────────────────────────────────────────────

// ── Benchmark: TraceEntry to JSON ───────────────────────────────────────────

func BenchmarkTraceEntryToJSON(b *testing.B) {
	te := makeTraceEntry(
		net.ParseIP("10.10.10.1"),
		net.ParseIP("10.10.10.2"),
		8080, 443, 6, 1234567890,
	)

	type jsonEntry struct {
		TraceID     string `json:"trace_id"`
		TimestampNS uint64 `json:"timestamp_ns"`
		SrcIP       string `json:"src_ip"`
		DstIP       string `json:"dst_ip"`
		SrcPort     uint16 `json:"src_port"`
		DstPort     uint16 `json:"dst_port"`
		Protocol    string `json:"protocol"`
		PacketLen   uint16 `json:"packet_len"`
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		je := jsonEntry{
			TraceID:     te.TraceIDHex(),
			TimestampNS: te.TimestampNS,
			SrcIP:       te.SrcIPAddr().String(),
			DstIP:       te.DstIPAddr().String(),
			SrcPort:     te.SrcPort,
			DstPort:     te.DstPort,
			Protocol:    te.ProtocolName(),
			PacketLen:   te.PacketLen,
		}
		_, err := json.Marshal(je)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ── Benchmark: FiveTuple reverse ────────────────────────────────────────────

func BenchmarkFiveTupleReverse(b *testing.B) {
	ft := FiveTuple{
		SrcIP:    [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 10, 0, 0, 1},
		DstIP:    [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 10, 0, 0, 2},
		SrcPort:  12345,
		DstPort:  80,
		Protocol: 6,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ft.Reverse()
	}
}

// ── Benchmark: PacketCorrelator.Add ─────────────────────────────────────────

func BenchmarkPacketCorrelatorAdd(b *testing.B) {
	pc := NewPacketCorrelator(DefaultPacketCorrelatorConfig())
	entries := make([]*TraceEntry, 1000)
	for i := range entries {
		entries[i] = makeTraceEntry(
			net.ParseIP("10.0.0.1"),
			net.ParseIP("10.0.0.2"),
			uint16(i%65536), 80, 6, uint64(i*1000),
		)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pc.Add(entries[i%len(entries)])
	}
}

// ── Benchmark: PacketCorrelator bidirectional match ─────────────────────────

func BenchmarkPacketCorrelatorBidiMatch(b *testing.B) {
	pc := NewPacketCorrelator(DefaultPacketCorrelatorConfig())

	// Pre-populate with forward entries
	for i := 0; i < 100; i++ {
		forward := makeTraceEntry(
			net.ParseIP("10.0.0.1"),
			net.ParseIP("10.0.0.2"),
			uint16(10000+i), 80, 6, uint64(i*1000),
		)
		pc.Add(forward)
	}

	// Benchmark adding reverse entries (triggering matches)
	reverseEntries := make([]*TraceEntry, 100)
	for i := range reverseEntries {
		reverseEntries[i] = makeTraceEntry(
			net.ParseIP("10.0.0.2"),
			net.ParseIP("10.0.0.1"),
			80, uint16(10000+i), 6, uint64(i*1000+500),
		)
	}

	// Drain output channel in background
	go func() {
		for range pc.CorrelatedFlows() {
		}
	}()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pc.Add(reverseEntries[i%len(reverseEntries)])
	}
}

// ── Benchmark: PacketCorrelator.GC ──────────────────────────────────────────

func BenchmarkPacketCorrelatorGC(b *testing.B) {
	config := DefaultPacketCorrelatorConfig()
	config.FlowTTL = 10 * time.Millisecond
	pc := NewPacketCorrelator(config)

	// Populate with many flows
	for i := 0; i < 10000; i++ {
		entry := makeTraceEntry(
			net.ParseIP("10.0.0.1"),
			net.ParseIP("10.0.0.2"),
			uint16(i%65536), uint16(80+i/65536), 6, 1000,
		)
		pc.Add(entry)
	}

	nowFarFuture := uint64(time.Now().Add(time.Hour).UnixNano())

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pc.GC(nowFarFuture)
		// Re-populate after GC clears everything
		if pc.Len() == 0 {
			b.StopTimer()
			for j := 0; j < 10000; j++ {
				entry := makeTraceEntry(
					net.ParseIP("10.0.0.1"),
					net.ParseIP("10.0.0.2"),
					uint16(j%65536), uint16(80+j/65536), 6, 1000,
				)
				pc.Add(entry)
			}
			b.StartTimer()
		}
	}
}

// ── Benchmark: Batch processing pipeline ────────────────────────────────────

func BenchmarkBatchProcessingPipeline(b *testing.B) {
	// Simulate the full pipeline: decode -> correlate -> JSON
	// The records arrive from the PACKET_EVENTS ring buffer (the only path).
	rawEntries := make([][]byte, 100)
	for i := range rawEntries {
		rawEntries[i] = encodeKernelPacketEvent(net.ParseIP("10.0.0.1"), net.ParseIP("10.0.0.2"),
			uint16(1000+i), 80, 6, 64)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		raw := rawEntries[i%len(rawEntries)]
		// Decode
		te, err := DecodeKernelPacketEvent(raw)
		if err != nil {
			b.Fatal(err)
		}
		// Convert to JSON
		type out struct {
			TraceID  string `json:"trace_id"`
			Protocol string `json:"protocol"`
			SrcPort  uint16 `json:"src_port"`
			DstPort  uint16 `json:"dst_port"`
		}
		_, err = json.Marshal(out{
			TraceID:  te.TraceIDHex(),
			Protocol: te.ProtocolName(),
			SrcPort:  te.SrcPort,
			DstPort:  te.DstPort,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ── Benchmark: StatsEntry decode ────────────────────────────────────────────

// ── Benchmark: one sweep of a full LATENCY_MAP ──────────────────────────────

// BenchmarkSweepLatency65536 is one sweep of a full map (65,536 entries, 17
// batches) against the fake map: the userspace half of the cost, with no
// system calls. "stale" is the steady state, where no thread took a new sample
// since the last sweep and nothing is published. "fresh" is the worst case,
// every entry new, so every one is decoded, marshalled and published.
func BenchmarkSweepLatency65536(b *testing.B) {
	const n = 65536
	keys, values := latencyEntries(n, LatencyTraceKeySize, 1)
	ctx := context.Background()

	b.Run("stale", func(b *testing.B) {
		m := &fakeBatchedMap{keys: keys, values: values, batchSize: latencySweepBatch}
		s := newLatencySweeper(m, func([]byte) {})
		if err := s.sweep(ctx); err != nil { // the first sweep sees every entry as new
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := s.sweep(ctx); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("fresh", func(b *testing.B) {
		m := &fakeBatchedMap{keys: keys, values: values, batchSize: latencySweepBatch}
		published := 0
		s := newLatencySweeper(m, func([]byte) { published++ })
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			s.rtts, s.lastSweepNS = newRTTTracker(), 0 // forget everything: each entry is new again
			if err := s.sweep(ctx); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		if published != n*b.N {
			b.Fatalf("published %d, want %d", published, n*b.N)
		}
	})
}

// ── Benchmark: one poll of a full FLOWS map ─────────────────────────────────

// BenchmarkFlowPoll65536 is one poll of a full FLOWS map (65,536 entries)
// against the fake map, with no system calls: the userspace half. Fifty flows
// are live; the rest went quiet ten minutes ago and were never deleted, which
// is what an LRU map of flows looks like after a day (west, 2026-11-15: 7,337
// entries, 54 live).
func BenchmarkFlowPoll65536(b *testing.B) {
	const n, live = 65536, 50
	now := ktimeNow()
	keys, values := make([][]byte, n), make([][]byte, n)
	for i := range uint32(n) {
		fk := benchFlowKey(i)
		lastSeen := now - uint64(10*time.Minute)
		if i < live {
			lastSeen = now
		}
		fs := makeFlowState(ConnStateEstablished, lastSeen, lastSeen, 1, 2, 100, 200)
		k, v := fk.Encode(), fs.Encode()
		keys[i], values[i] = k[:], v[:]
	}
	loader := &batchMockLoader{
		MockBPFLoader: NewMockBPFLoader(),
		flows:         &batchMockMap{MockMapReader: NewMockMapReader(), fake: &fakeBatchedMap{keys: keys, values: values, batchSize: mapSweepBatch}},
	}
	fr := NewFlowReader(loader, nil, DefaultFlowReaderConfig())
	ctx := context.Background()
	fr.pollOnce(ctx) // the first poll meets every flow

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fr.pollOnce(ctx)
	}
	b.StopTimer()
	if got := fr.ActiveFlows(); got != live {
		b.Fatalf("active flows = %d, want %d", got, live)
	}
	b.ReportMetric(float64(fr.Stats().EventsSent)/float64(b.N+1), "events/poll")
}

// ── Benchmark: the kernel's half of one pass ────────────────────────────────

// BenchmarkKernelBatchPass65536 is one batched pass over a full kernel LRU
// hash of FLOWS's shape, with a visit that does nothing: what the bpf() calls
// themselves cost on this kernel. Needs root.
//
//	go test -c -o /tmp/tc.test ./cmd/trace-collector-go && sudo /tmp/tc.test -test.run '^$' -test.bench KernelBatchPass
func BenchmarkKernelBatchPass65536(b *testing.B) {
	if os.Geteuid() != 0 {
		b.Skip("needs root (CAP_BPF)")
	}
	const n = 65536
	km, err := ciliumebpf.NewMap(&ciliumebpf.MapSpec{
		Type: ciliumebpf.LRUHash, KeySize: FlowKeySize, ValueSize: FlowStateSize, MaxEntries: n,
	})
	if err != nil {
		b.Fatal(err)
	}
	defer km.Close()
	for i := range uint32(n) {
		fk := benchFlowKey(i)
		fs := makeFlowState(ConnStateEstablished, 1, 1, 1, 2, 100, 200)
		k, v := fk.Encode(), fs.Encode()
		if err := km.Put(k[:], v[:]); err != nil {
			b.Fatal(err)
		}
	}
	w := newMapWalker("flows", newEBPFBatchMap(km,
		func(k *[FlowKeySize]byte) []byte { return k[:] },
		func(v *[FlowStateSize]byte) []byte { return v[:] }))
	ctx := context.Background()
	entries := 0
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entries = 0
		if err := w.read(ctx, func(_, _ []byte) { entries++ }); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(entries), "entries/pass")
}

// benchFlowKey is the i-th benchmark flow: source 10.x.y.z holding i in its
// low 24 bits, source port the low 16 bits of i. Built from bytes, so no
// narrowing conversion (gosec G115) is needed.
func benchFlowKey(i uint32) FlowKey {
	var src [4]byte
	binary.BigEndian.PutUint32(src[:], 10<<24|i)
	return makeFlowKey(src, [4]byte{10, 20, 0, 1}, binary.BigEndian.Uint16(src[2:]), 443, 6)
}
