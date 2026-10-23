// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"encoding/binary"
	"encoding/json"
	"testing"
)

// kernelPacketEvent lays out packet_marker's PacketEvent (ebpf/common,
// repr(C)) as the kernel writes it into PACKET_EVENTS.
func kernelPacketEvent() []byte {
	b := make([]byte, KernelPacketEventSize)
	binary.LittleEndian.PutUint64(b[0:], 123456789)           // timestamp_ns
	binary.LittleEndian.PutUint64(b[8:], 0x1111222233334444)  // trace_id.high
	binary.LittleEndian.PutUint64(b[16:], 0x5555666677778888) // trace_id.low
	copy(b[24:28], []byte{172, 28, 1, 5})                     // src_addr, network order
	copy(b[28:32], []byte{172, 28, 1, 1})                     // dst_addr
	binary.BigEndian.PutUint16(b[32:], 43210)                 // src_port, network order
	binary.BigEndian.PutUint16(b[34:], 18000)                 // dst_port
	b[36] = 6                                                 // protocol
	binary.LittleEndian.PutUint32(b[40:], 1514)               // packet_len
	b[44] = 2                                                 // action: Marked
	b[45] = 1                                                 // direction: Egress
	return b
}

// The ring buffer carries PacketEvents, 48 bytes. The reader decoded them as
// 68-byte TraceEntries, so every record failed ("too short") and no real
// packet ever left the collector (646 decode errors in a minute, live).
func TestDecodeKernelPacketEvent(t *testing.T) {
	te, err := DecodeKernelPacketEvent(kernelPacketEvent())
	if err != nil {
		t.Fatal(err)
	}
	if te.TimestampNS != 123456789 || te.TraceIDHex() != "11112222333344445555666677778888" {
		t.Errorf("ts %d trace %s", te.TimestampNS, te.TraceIDHex())
	}
	if te.SrcIPAddr().String() != "172.28.1.5" || te.DstIPAddr().String() != "172.28.1.1" {
		t.Errorf("addrs %s -> %s", te.SrcIPAddr(), te.DstIPAddr())
	}
	if te.SrcPort != 43210 || te.DstPort != 18000 || te.Protocol != 6 || te.PacketLen != 1514 {
		t.Errorf("ports %d->%d proto %d len %d", te.SrcPort, te.DstPort, te.Protocol, te.PacketLen)
	}
	if te.Action != "marked" || te.Direction != "egress" {
		t.Errorf("action %q direction %q", te.Action, te.Direction)
	}
	if _, err := DecodeKernelPacketEvent(make([]byte, KernelPacketEventSize-1)); err == nil {
		t.Error("short record decoded")
	}
}

// ebpf.packet.events is parsed by the dashboard as its PacketEvent schema;
// it used to receive the traces.packet shape (trace_id as a hex string,
// src_ip/dst_ip, no flow_key), which the dashboard cannot parse.
func TestMarshalDashboardPacket(t *testing.T) {
	te, _ := DecodeKernelPacketEvent(kernelPacketEvent())
	raw, err := marshalDashboardPacket(te)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		TimestampNs uint64 `json:"timestamp_ns"`
		TraceID     struct {
			High uint64 `json:"high"`
			Low  uint64 `json:"low"`
		} `json:"trace_id"`
		FlowKey struct {
			SrcAddr  string `json:"src_addr"`
			DstAddr  string `json:"dst_addr"`
			SrcPort  uint16 `json:"src_port"`
			DstPort  uint16 `json:"dst_port"`
			Protocol uint8  `json:"protocol"`
		} `json:"flow_key"`
		PacketLen uint32 `json:"packet_len"`
		Action    string `json:"action"`
		Direction string `json:"direction"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.TimestampNs != 123456789 || got.TraceID.High != 0x1111222233334444 || got.TraceID.Low != 0x5555666677778888 ||
		got.FlowKey.SrcAddr != "172.28.1.5" || got.FlowKey.DstAddr != "172.28.1.1" ||
		got.FlowKey.SrcPort != 43210 || got.FlowKey.DstPort != 18000 || got.FlowKey.Protocol != 6 ||
		got.PacketLen != 1514 || got.Action != "marked" || got.Direction != "egress" {
		t.Errorf("dashboard packet = %+v", got)
	}
}

// Flow events use the dashboard's ebpf.FlowEvent field names, trace_id
// {high, low} included: on east (2026-10-23) it went out as {"High","Low"},
// which Go's case-insensitive decoder hid and any other reader misses.
func TestMarshalDashboardFlow(t *testing.T) {
	ev := &FlowEventEntry{
		TimestampNS: 857754687850080,
		Key:         FlowKey{SrcAddr: 0x7f000001, DstAddr: 0x7f000001, SrcPort: 19005, DstPort: 51738, Protocol: 6},
		State: FlowStateEntry{StartNS: 857754687850080, LastSeenNS: 860868687778555,
			PacketsIn: 3, PacketsOut: 3, BytesIn: 222, BytesOut: 162, State: ConnStateFinWait1},
		EventType: FlowEventStateChange,
	}
	binary.BigEndian.PutUint64(ev.State.TraceID[0:8], 0x0192a3b4c5d6e7f8)
	binary.BigEndian.PutUint64(ev.State.TraceID[8:16], 0x1122334455667788)
	raw, err := marshalDashboardFlow(ev)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	fk, _ := got["flow_key"].(map[string]any)
	fs, _ := got["flow_state"].(map[string]any)
	tid, _ := fs["trace_id"].(map[string]any)
	if got["timestamp_ns"] != float64(857754687850080) || got["event_type"] != "state_change" ||
		fk["src_addr"] != "127.0.0.1" || fk["dst_addr"] != "127.0.0.1" || fk["src_port"] != float64(19005) ||
		fk["dst_port"] != float64(51738) || fk["protocol"] != float64(6) ||
		fs["state"] != "fin_wait1" || fs["packets_in"] != float64(3) || fs["bytes_out"] != float64(162) ||
		fs["start_ns"] != float64(857754687850080) || fs["last_seen_ns"] != float64(860868687778555) {
		t.Errorf("dashboard flow = %s", raw)
	}
	if tid["high"] != float64(0x0192a3b4c5d6e7f8) || tid["low"] != float64(0x1122334455667788) {
		t.Errorf("flow_state.trace_id = %v, want {high, low}", fs["trace_id"])
	}
}
