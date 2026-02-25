// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// types.go — Shared types for the packet-marker BPF program's map entries.
//
// These Go structs mirror the Rust types in ebpf/common/src/lib.rs byte-for-byte.
// They define the layout of BPF map entries produced by the packet_marker XDP
// program and consumed by this trace collector.
//
// Per RFC 9669: we use "BPF" (not "eBPF") throughout.
package main

import (
	"encoding/binary"
	"fmt"
	"net"
)

// ── TraceEntry ──────────────────────────────────────────────────────────────
// TraceEntry matches the BPF TRACE_MAP entry layout written by packet_marker.
// Size: 16 + 8 + 16 + 16 + 2 + 2 + 1 + 1 + 2 + 1 + 3 = 68 bytes.
type TraceEntry struct {
	TraceID     [16]byte // UUIDv7 trace identifier (128-bit)
	TimestampNS uint64   // ktime_get_ns() kernel timestamp
	SrcIP       [16]byte // Source address (IPv4-mapped-IPv6 or native IPv6)
	DstIP       [16]byte // Destination address
	SrcPort     uint16   // Source port (network byte order)
	DstPort     uint16   // Destination port (network byte order)
	Protocol    uint8    // IP protocol (TCP=6, UDP=17, ICMP=58)
	Flags       uint8    // TCP flags or ICMP type
	PacketLen   uint16   // Total packet length in bytes
	HopCount    uint8    // Number of hops observed
	Pad         [3]byte  // Alignment padding
}

// TraceEntrySize is the expected wire size of a TraceEntry.
const TraceEntrySize = 68

// DecodeTraceEntry decodes a TraceEntry from a byte slice (little-endian).
func DecodeTraceEntry(b []byte) (*TraceEntry, error) {
	if len(b) < TraceEntrySize {
		return nil, fmt.Errorf("trace entry too short: %d < %d", len(b), TraceEntrySize)
	}
	te := &TraceEntry{}
	copy(te.TraceID[:], b[0:16])
	te.TimestampNS = binary.LittleEndian.Uint64(b[16:24])
	copy(te.SrcIP[:], b[24:40])
	copy(te.DstIP[:], b[40:56])
	te.SrcPort = binary.BigEndian.Uint16(b[56:58])
	te.DstPort = binary.BigEndian.Uint16(b[58:60])
	te.Protocol = b[60]
	te.Flags = b[61]
	te.PacketLen = binary.LittleEndian.Uint16(b[62:64])
	te.HopCount = b[64]
	copy(te.Pad[:], b[65:68])
	return te, nil
}

// Encode serializes a TraceEntry to a byte slice.
func (te *TraceEntry) Encode() [TraceEntrySize]byte {
	var b [TraceEntrySize]byte
	copy(b[0:16], te.TraceID[:])
	binary.LittleEndian.PutUint64(b[16:24], te.TimestampNS)
	copy(b[24:40], te.SrcIP[:])
	copy(b[40:56], te.DstIP[:])
	binary.BigEndian.PutUint16(b[56:58], te.SrcPort)
	binary.BigEndian.PutUint16(b[58:60], te.DstPort)
	b[60] = te.Protocol
	b[61] = te.Flags
	binary.LittleEndian.PutUint16(b[62:64], te.PacketLen)
	b[64] = te.HopCount
	copy(b[65:68], te.Pad[:])
	return b
}

// TraceIDHex returns the trace ID as a hex string.
func (te *TraceEntry) TraceIDHex() string {
	return fmt.Sprintf("%032x", te.TraceID)
}

// SrcIPAddr returns the source IP as a net.IP.
func (te *TraceEntry) SrcIPAddr() net.IP {
	return net.IP(te.SrcIP[:])
}

// DstIPAddr returns the destination IP as a net.IP.
func (te *TraceEntry) DstIPAddr() net.IP {
	return net.IP(te.DstIP[:])
}

// ProtocolName returns the human-readable protocol name.
func (te *TraceEntry) ProtocolName() string {
	switch te.Protocol {
	case 6:
		return "TCP"
	case 17:
		return "UDP"
	case 58:
		return "ICMPv6"
	case 1:
		return "ICMP"
	default:
		return fmt.Sprintf("proto(%d)", te.Protocol)
	}
}

// FiveTuple returns a FiveTuple key for this trace entry.
func (te *TraceEntry) FiveTuple() FiveTuple {
	return FiveTuple{
		SrcIP:    te.SrcIP,
		DstIP:    te.DstIP,
		SrcPort:  te.SrcPort,
		DstPort:  te.DstPort,
		Protocol: te.Protocol,
	}
}

// ── StatsEntry ──────────────────────────────────────────────────────────────
// StatsEntry matches the BPF STATS map layout from packet_marker.
type StatsEntry struct {
	PacketsTotal  uint64 // Total packets seen
	BytesTotal    uint64 // Total bytes seen
	TracesCreated uint64 // New trace IDs generated
	TracesUpdated uint64 // Existing traces updated with new info
	Errors        uint64 // Processing errors (parse failures, etc.)
}

// StatsEntrySize is the expected wire size of a StatsEntry.
const StatsEntrySize = 40

// DecodeStatsEntry decodes a StatsEntry from a byte slice (little-endian).
func DecodeStatsEntry(b []byte) (*StatsEntry, error) {
	if len(b) < StatsEntrySize {
		return nil, fmt.Errorf("stats entry too short: %d < %d", len(b), StatsEntrySize)
	}
	return &StatsEntry{
		PacketsTotal:  binary.LittleEndian.Uint64(b[0:8]),
		BytesTotal:    binary.LittleEndian.Uint64(b[8:16]),
		TracesCreated: binary.LittleEndian.Uint64(b[16:24]),
		TracesUpdated: binary.LittleEndian.Uint64(b[24:32]),
		Errors:        binary.LittleEndian.Uint64(b[32:40]),
	}, nil
}

// ── FiveTuple ───────────────────────────────────────────────────────────────
// FiveTuple is a connection identifier used as a map key for flow correlation.
type FiveTuple struct {
	SrcIP    [16]byte
	DstIP    [16]byte
	SrcPort  uint16
	DstPort  uint16
	Protocol uint8
}

// String returns a human-readable representation of the 5-tuple.
func (ft FiveTuple) String() string {
	srcIP := net.IP(ft.SrcIP[:])
	dstIP := net.IP(ft.DstIP[:])
	proto := "unknown"
	switch ft.Protocol {
	case 6:
		proto = "tcp"
	case 17:
		proto = "udp"
	}
	return fmt.Sprintf("%s:%d->%s:%d/%s", srcIP, ft.SrcPort, dstIP, ft.DstPort, proto)
}

// Reverse returns the reverse direction 5-tuple (swap src/dst).
func (ft FiveTuple) Reverse() FiveTuple {
	return FiveTuple{
		SrcIP:    ft.DstIP,
		DstIP:    ft.SrcIP,
		SrcPort:  ft.DstPort,
		DstPort:  ft.SrcPort,
		Protocol: ft.Protocol,
	}
}
