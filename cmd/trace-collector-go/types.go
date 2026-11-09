// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// types.go — Shared types for packets the packet_marker BPF program reports.
//
// Packet records arrive only from the PACKET_EVENTS ring buffer and are decoded
// into TraceEntry by DecodeKernelPacketEvent (packet_event.go). The 68-byte
// TRACE_MAP and STATS decoders left with the poll path (2026-11-09).
//
// Per RFC 9669: we use "BPF" (not "eBPF") throughout.
package main

import (
	"fmt"
	"net"
)

// ── TraceEntry ──────────────────────────────────────────────────────────────

// TraceEntry is one packet as the collector carries it, decoded from a
// PACKET_EVENTS record.
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

	// From packet_marker's PacketEvent (DecodeKernelPacketEvent); not part
	// of the 68-byte encoding. "pass", "marked", ...; "ingress"/"egress".
	Action    string
	Direction string
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
