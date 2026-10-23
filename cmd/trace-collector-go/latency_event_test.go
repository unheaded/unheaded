// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"net"
	"testing"
	"time"
)

// latency-probe keys LATENCY_MAP by (zero trace ID, pid&0xffff, tid&0xffff),
// not by 5-tuple. On east (2026-10-23) every traces.latency record carried
// flow_key {src_addr "::", dst_addr "::", src_port 18570, dst_port 18654}:
// a flow between no addresses whose "ports" were a pid and a tid.
func TestLatencyMapEvent_NoFlowKeyWithoutAddresses(t *testing.T) {
	lk := &LatencyKey{SrcPort: 18570, DstPort: 18654, Protocol: 6}
	le := &LatencyEntry{RTTNS: 98879, MinRTTNS: 678, MaxRTTNS: 5460725, Samples: 607968}
	now := time.Unix(1792734554, 213496086)

	evt := latencyMapEvent(lk, le, now)
	if fk, ok := evt["flow_key"]; ok {
		t.Errorf("flow_key = %v, want none: the map key holds no addresses", fk)
	}
	if evt["latency_ns"] != uint64(98879) || evt["samples"] != uint64(607968) {
		t.Errorf("latency_ns/samples = %v/%v, want 98879/607968", evt["latency_ns"], evt["samples"])
	}
	if evt["timestamp_ns"] != uint64(now.UnixNano()) {
		t.Errorf("timestamp_ns = %v, want wall clock %d", evt["timestamp_ns"], now.UnixNano())
	}
	if evt["pid"] != uint32(0) || evt["tid"] != uint32(0) {
		t.Errorf("pid/tid = %v/%v, want 0/0 (the key holds only their low 16 bits)", evt["pid"], evt["tid"])
	}

	// A key that does hold addresses keeps its flow_key.
	copy(lk.SrcIP[:], net.ParseIP("10.0.0.1").To16())
	copy(lk.DstIP[:], net.ParseIP("10.0.0.2").To16())
	fk, ok := latencyMapEvent(lk, le, now)["flow_key"].(map[string]interface{})
	if !ok || fk["src_addr"] != "10.0.0.1" || fk["dst_addr"] != "10.0.0.2" || fk["src_port"] != uint16(18570) {
		t.Errorf("flow_key = %v, want 10.0.0.1:18570 -> 10.0.0.2", fk)
	}
}
