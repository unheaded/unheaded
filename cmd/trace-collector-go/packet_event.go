// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
)

// KernelPacketEventSize is the size of packet_marker's PacketEvent
// (ebpf/common/src/lib.rs, repr(C)): timestamp_ns u64 @0, trace_id
// {high,low u64} @8, flow_key {src,dst u32; sport,dport u16; proto u8;
// 3 pad} @24, packet_len u32 @40, action u8 @44, direction u8 @45, 2 pad.
const KernelPacketEventSize = 48

var packetActions = []string{"pass", "drop", "marked", "extracted", "redirect"}

// DecodeKernelPacketEvent decodes one PACKET_EVENTS ring-buffer record into
// a TraceEntry. Addresses and ports arrive in network byte order; the IPv4
// addresses become IPv4-mapped IPv6 as TraceEntry stores them.
func DecodeKernelPacketEvent(b []byte) (*TraceEntry, error) {
	if len(b) < KernelPacketEventSize {
		return nil, fmt.Errorf("packet event too short: %d < %d", len(b), KernelPacketEventSize)
	}
	te := &TraceEntry{TimestampNS: binary.LittleEndian.Uint64(b[0:8])}
	binary.BigEndian.PutUint64(te.TraceID[0:8], binary.LittleEndian.Uint64(b[8:16]))
	binary.BigEndian.PutUint64(te.TraceID[8:16], binary.LittleEndian.Uint64(b[16:24]))
	for _, ip := range []*[16]byte{&te.SrcIP, &te.DstIP} {
		ip[10], ip[11] = 0xff, 0xff
	}
	copy(te.SrcIP[12:16], b[24:28])
	copy(te.DstIP[12:16], b[28:32])
	te.SrcPort = binary.BigEndian.Uint16(b[32:34])
	te.DstPort = binary.BigEndian.Uint16(b[34:36])
	te.Protocol = b[36]
	te.PacketLen = uint16(min(binary.LittleEndian.Uint32(b[40:44]), math.MaxUint16)) // #nosec G115 -- clamped to uint16 on the line itself
	if a := int(b[44]); a < len(packetActions) {
		te.Action = packetActions[a]
	} else {
		te.Action = fmt.Sprintf("action_%d", a)
	}
	te.Direction = "ingress"
	if b[45] == 1 {
		te.Direction = "egress"
	}
	return te, nil
}

// marshalDashboardPacket renders a packet in the schema the dashboard's
// ingestor parses from ebpf.packet.events (its ebpf.PacketEvent).
func marshalDashboardPacket(te *TraceEntry) ([]byte, error) {
	type traceID struct {
		High uint64 `json:"high"`
		Low  uint64 `json:"low"`
	}
	type flowKey struct {
		SrcAddr  string `json:"src_addr"`
		DstAddr  string `json:"dst_addr"`
		SrcPort  uint16 `json:"src_port"`
		DstPort  uint16 `json:"dst_port"`
		Protocol uint8  `json:"protocol"`
	}
	action, direction := te.Action, te.Direction
	if action == "" {
		action = "pass"
	}
	if direction == "" {
		direction = "ingress"
	}
	return json.Marshal(struct {
		TimestampNs uint64  `json:"timestamp_ns"`
		TraceID     traceID `json:"trace_id"`
		FlowKey     flowKey `json:"flow_key"`
		PacketLen   uint32  `json:"packet_len"`
		Action      string  `json:"action"`
		Direction   string  `json:"direction"`
	}{
		TimestampNs: te.TimestampNS,
		TraceID:     traceID{binary.BigEndian.Uint64(te.TraceID[0:8]), binary.BigEndian.Uint64(te.TraceID[8:16])},
		FlowKey:     flowKey{te.SrcIPAddr().String(), te.DstIPAddr().String(), te.SrcPort, te.DstPort, te.Protocol},
		PacketLen:   uint32(te.PacketLen),
		Action:      action,
		Direction:   direction,
	})
}
