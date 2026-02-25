// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"encoding/json"
	"net"
	"testing"
	"time"
)

// ── Benchmark: TraceEntry decode ────────────────────────────────────────────

func BenchmarkDecodeTraceEntry(b *testing.B) {
	te := makeTraceEntry(
		net.ParseIP("10.10.10.1"),
		net.ParseIP("10.10.10.2"),
		8080, 443, 6, 1234567890,
	)
	encoded := te.Encode()
	data := encoded[:]

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := DecodeTraceEntry(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ── Benchmark: TraceEntry encode ────────────────────────────────────────────

func BenchmarkEncodeTraceEntry(b *testing.B) {
	te := makeTraceEntry(
		net.ParseIP("10.10.10.1"),
		net.ParseIP("10.10.10.2"),
		8080, 443, 6, 1234567890,
	)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = te.Encode()
	}
}

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
	rawEntries := make([][TraceEntrySize]byte, 100)
	for i := range rawEntries {
		te := makeTraceEntry(
			net.ParseIP("10.0.0.1"),
			net.ParseIP("10.0.0.2"),
			uint16(1000+i), 80, 6, uint64(i*1000),
		)
		rawEntries[i] = te.Encode()
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		raw := rawEntries[i%len(rawEntries)]
		// Decode
		te, err := DecodeTraceEntry(raw[:])
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

func BenchmarkDecodeStatsEntry(b *testing.B) {
	var raw [StatsEntrySize]byte
	for i := range raw {
		raw[i] = byte(i)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := DecodeStatsEntry(raw[:])
		if err != nil {
			b.Fatal(err)
		}
	}
}
