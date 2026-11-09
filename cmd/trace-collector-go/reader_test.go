// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"
)

// ── Helper: create a trace entry with known values ──────────────────────────

func makeTraceEntry(srcIP, dstIP net.IP, srcPort, dstPort uint16, proto uint8, tsNS uint64) *TraceEntry {
	te := &TraceEntry{
		TimestampNS: tsNS,
		SrcPort:     srcPort,
		DstPort:     dstPort,
		Protocol:    proto,
		PacketLen:   128,
		HopCount:    1,
	}
	// Set trace ID (synthetic)
	binary.BigEndian.PutUint64(te.TraceID[0:8], tsNS)
	binary.BigEndian.PutUint64(te.TraceID[8:16], uint64(srcPort)<<16|uint64(dstPort))

	// Copy IPs into 16-byte fields (IPv4-mapped-IPv6)
	src16 := srcIP.To16()
	dst16 := dstIP.To16()
	if src16 != nil {
		copy(te.SrcIP[:], src16)
	}
	if dst16 != nil {
		copy(te.DstIP[:], dst16)
	}
	return te
}

// ── TraceEntry encode/decode roundtrip ──────────────────────────────────────

func TestTraceEntry_EncodeDecodeRoundtrip(t *testing.T) {
	original := makeTraceEntry(
		net.ParseIP("10.10.10.1"),
		net.ParseIP("10.10.10.2"),
		8080, 443, 6, 1000000,
	)

	encoded := original.Encode()
	decoded, err := DecodeTraceEntry(encoded[:])
	if err != nil {
		t.Fatalf("DecodeTraceEntry: %v", err)
	}

	if decoded.TimestampNS != original.TimestampNS {
		t.Errorf("TimestampNS = %d, want %d", decoded.TimestampNS, original.TimestampNS)
	}
	if decoded.SrcPort != original.SrcPort {
		t.Errorf("SrcPort = %d, want %d", decoded.SrcPort, original.SrcPort)
	}
	if decoded.DstPort != original.DstPort {
		t.Errorf("DstPort = %d, want %d", decoded.DstPort, original.DstPort)
	}
	if decoded.Protocol != original.Protocol {
		t.Errorf("Protocol = %d, want %d", decoded.Protocol, original.Protocol)
	}
	if decoded.PacketLen != original.PacketLen {
		t.Errorf("PacketLen = %d, want %d", decoded.PacketLen, original.PacketLen)
	}
	if decoded.HopCount != original.HopCount {
		t.Errorf("HopCount = %d, want %d", decoded.HopCount, original.HopCount)
	}
	if decoded.TraceID != original.TraceID {
		t.Errorf("TraceID mismatch")
	}
	if decoded.SrcIP != original.SrcIP {
		t.Errorf("SrcIP mismatch")
	}
	if decoded.DstIP != original.DstIP {
		t.Errorf("DstIP mismatch")
	}
}

func TestDecodeTraceEntry_TooShort(t *testing.T) {
	_, err := DecodeTraceEntry(make([]byte, 10))
	if err == nil {
		t.Error("expected error for short input")
	}
}

func TestTraceEntry_TraceIDHex(t *testing.T) {
	te := &TraceEntry{}
	te.TraceID[0] = 0xAB
	te.TraceID[15] = 0xCD

	hex := te.TraceIDHex()
	if len(hex) != 32 {
		t.Errorf("TraceIDHex length = %d, want 32", len(hex))
	}
	// First byte AB, last byte CD
	if hex[:2] != "ab" {
		t.Errorf("TraceIDHex starts with %q, want \"ab\"", hex[:2])
	}
	if hex[30:32] != "cd" {
		t.Errorf("TraceIDHex ends with %q, want \"cd\"", hex[30:32])
	}
}

func TestTraceEntry_ProtocolName(t *testing.T) {
	tests := []struct {
		proto uint8
		want  string
	}{
		{6, "TCP"},
		{17, "UDP"},
		{58, "ICMPv6"},
		{1, "ICMP"},
		{99, "proto(99)"},
	}
	for _, tt := range tests {
		te := &TraceEntry{Protocol: tt.proto}
		if got := te.ProtocolName(); got != tt.want {
			t.Errorf("ProtocolName(%d) = %q, want %q", tt.proto, got, tt.want)
		}
	}
}

func TestTraceEntry_FiveTuple(t *testing.T) {
	te := makeTraceEntry(
		net.ParseIP("10.0.0.1"),
		net.ParseIP("10.0.0.2"),
		1234, 5678, 6, 0,
	)

	ft := te.FiveTuple()
	if ft.SrcPort != 1234 {
		t.Errorf("FiveTuple.SrcPort = %d, want 1234", ft.SrcPort)
	}
	if ft.DstPort != 5678 {
		t.Errorf("FiveTuple.DstPort = %d, want 5678", ft.DstPort)
	}
	if ft.Protocol != 6 {
		t.Errorf("FiveTuple.Protocol = %d, want 6", ft.Protocol)
	}
}

// ── StatsEntry tests ────────────────────────────────────────────────────────

func TestDecodeStatsEntry(t *testing.T) {
	var b [StatsEntrySize]byte
	binary.LittleEndian.PutUint64(b[0:8], 1000)
	binary.LittleEndian.PutUint64(b[8:16], 50000)
	binary.LittleEndian.PutUint64(b[16:24], 100)
	binary.LittleEndian.PutUint64(b[24:32], 50)
	binary.LittleEndian.PutUint64(b[32:40], 5)

	stats, err := DecodeStatsEntry(b[:])
	if err != nil {
		t.Fatalf("DecodeStatsEntry: %v", err)
	}

	if stats.PacketsTotal != 1000 {
		t.Errorf("PacketsTotal = %d, want 1000", stats.PacketsTotal)
	}
	if stats.BytesTotal != 50000 {
		t.Errorf("BytesTotal = %d, want 50000", stats.BytesTotal)
	}
	if stats.TracesCreated != 100 {
		t.Errorf("TracesCreated = %d, want 100", stats.TracesCreated)
	}
	if stats.TracesUpdated != 50 {
		t.Errorf("TracesUpdated = %d, want 50", stats.TracesUpdated)
	}
	if stats.Errors != 5 {
		t.Errorf("Errors = %d, want 5", stats.Errors)
	}
}

func TestDecodeStatsEntry_TooShort(t *testing.T) {
	_, err := DecodeStatsEntry(make([]byte, 10))
	if err == nil {
		t.Error("expected error for short input")
	}
}

// ── FiveTuple tests ─────────────────────────────────────────────────────────

func TestFiveTuple_String(t *testing.T) {
	ft := FiveTuple{
		SrcPort:  8080,
		DstPort:  443,
		Protocol: 6,
	}
	// IPv4-mapped IPv6 zeros
	s := ft.String()
	if s == "" {
		t.Error("String() returned empty")
	}
}

func TestFiveTuple_Reverse(t *testing.T) {
	ft := FiveTuple{
		SrcIP:    [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 10, 0, 0, 1},
		DstIP:    [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 10, 0, 0, 2},
		SrcPort:  1234,
		DstPort:  5678,
		Protocol: 6,
	}

	rev := ft.Reverse()
	if rev.SrcIP != ft.DstIP {
		t.Error("Reverse SrcIP != original DstIP")
	}
	if rev.DstIP != ft.SrcIP {
		t.Error("Reverse DstIP != original SrcIP")
	}
	if rev.SrcPort != ft.DstPort {
		t.Error("Reverse SrcPort != original DstPort")
	}
	if rev.DstPort != ft.SrcPort {
		t.Error("Reverse DstPort != original SrcPort")
	}
	if rev.Protocol != ft.Protocol {
		t.Error("Reverse changed protocol")
	}
}

// ── TraceReader tests ───────────────────────────────────────────────────────

func TestTraceReader_RingbufMode(t *testing.T) {
	ringbufCh := make(chan []byte, 10)
	reader := NewTraceReader(DefaultTraceReaderConfig())

	// Send kernel PacketEvents (what PACKET_EVENTS carries) through the ringbuf channel
	ringbufCh <- encodeKernelPacketEvent(net.ParseIP("10.0.0.1"), net.ParseIP("10.0.0.2"), 8080, 443, 6, 1000)
	ringbufCh <- encodeKernelPacketEvent(net.ParseIP("10.0.0.3"), net.ParseIP("10.0.0.4"), 9090, 80, 6, 2000)

	ctx, cancel := context.WithCancel(context.Background())

	// Run in background
	done := make(chan struct{})
	go func() {
		reader.Run(ctx, ringbufCh)
		close(done)
	}()

	// Wait for events on the output channel
	var received []*TraceEntry
	timeout := time.After(2 * time.Second)
	for len(received) < 2 {
		select {
		case entry := <-reader.Events():
			received = append(received, entry)
		case <-timeout:
			t.Fatalf("timeout: received %d/2 events", len(received))
		}
	}

	// Verify entries
	if received[0].SrcPort != 8080 {
		t.Errorf("entry[0].SrcPort = %d, want 8080", received[0].SrcPort)
	}
	if received[1].SrcPort != 9090 {
		t.Errorf("entry[1].SrcPort = %d, want 9090", received[1].SrcPort)
	}

	stats := reader.Stats()
	if stats.EntriesRead != 2 {
		t.Errorf("EntriesRead = %d, want 2", stats.EntriesRead)
	}

	cancel()
	<-done
}

func TestTraceReader_RingbufBadData(t *testing.T) {
	ringbufCh := make(chan []byte, 10)
	reader := NewTraceReader(DefaultTraceReaderConfig())

	// Send bad data followed by good data
	ringbufCh <- []byte("tooshort")
	ringbufCh <- encodeKernelPacketEvent(net.ParseIP("10.0.0.1"), net.ParseIP("10.0.0.2"), 4444, 80, 6, 999)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		reader.Run(ctx, ringbufCh)
		close(done)
	}()

	// Should get the good entry despite the bad one
	select {
	case entry := <-reader.Events():
		if entry.SrcPort != 4444 {
			t.Errorf("SrcPort = %d, want 4444", entry.SrcPort)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	stats := reader.Stats()
	if stats.DecodeErrors != 1 {
		t.Errorf("DecodeErrors = %d, want 1", stats.DecodeErrors)
	}

	cancel()
	<-done
}

func TestTraceReader_RingbufChannelClose(t *testing.T) {
	ringbufCh := make(chan []byte, 10)
	reader := NewTraceReader(DefaultTraceReaderConfig())

	ctx := context.Background()
	done := make(chan struct{})
	go func() {
		reader.Run(ctx, ringbufCh)
		close(done)
	}()

	// Close the channel — reader should exit cleanly
	close(ringbufCh)

	select {
	case <-done:
		// Good — reader exited
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: reader didn't exit after channel close")
	}
}

// Ringbuf or nothing: with no PACKET_EVENTS ring buffer there is no poll
// fallback to drop to (it read packet_marker's 8-byte STATS counters as
// 68-byte traces and published none), so the start is refused, saying why.
func TestOpenPacketEvents_RefusesWithoutRingbuf(t *testing.T) {
	loader := NewMockBPFLoader() // packetEventsCh nil: no ring buffer
	ch, err := openPacketEvents(context.Background(), loader)
	if err == nil {
		t.Fatal("no ring buffer, and no error")
	}
	if ch != nil {
		t.Error("returned a channel along with the error")
	}
	for _, want := range []string{"PACKET_EVENTS", "--enable-packet-marker=false"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestOpenPacketEvents_RingbufAvailable(t *testing.T) {
	loader := NewMockBPFLoader()
	loader.packetEventsCh = make(chan []byte)
	ch, err := openPacketEvents(context.Background(), loader)
	if err != nil {
		t.Fatalf("openPacketEvents: %v", err)
	}
	if ch == nil {
		t.Fatal("nil channel with no error")
	}
}

// encodeKernelPacketEvent lays out packet_marker's 48-byte PacketEvent.
func encodeKernelPacketEvent(src, dst net.IP, sport, dport uint16, proto uint8, length uint32) []byte {
	b := make([]byte, KernelPacketEventSize)
	copy(b[24:28], src.To4())
	copy(b[28:32], dst.To4())
	binary.BigEndian.PutUint16(b[32:], sport)
	binary.BigEndian.PutUint16(b[34:], dport)
	b[36] = proto
	binary.LittleEndian.PutUint32(b[40:], length)
	return b
}
