// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"os"
	"testing"

	ciliumebpf "github.com/cilium/ebpf"
)

// builtSpec parses the object an environment variable names; unset skips,
// a set variable naming a missing or unreadable file fails.
func builtSpec(t *testing.T, env string) *ciliumebpf.CollectionSpec {
	t.Helper()
	path := os.Getenv(env)
	if path == "" {
		t.Skipf("set %s to a built object to check its maps", env)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("%s: %v", env, err)
	}
	defer f.Close()
	spec, err := ciliumebpf.LoadCollectionSpecFromReader(f)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return spec
}

// ADR-147 decision 19 (D7 = A): packet-marker's FLOW_STATE is an LRU, so a
// full map evicts instead of refusing every new flow (east's was full;
// west's filled in about 30 h), and STATS has room for key 8, the counted
// failed insert.
//
//	PACKET_MARKER_OBJ=$PWD/ebpf/target/load-gate/default/bpfel-unknown-none/release/packet-marker \
//	    go test ./cmd/trace-collector-go -run TestBuiltPacketMarkerFlowStateIsLRU -count=1
func TestBuiltPacketMarkerFlowStateIsLRU(t *testing.T) {
	spec := builtSpec(t, "PACKET_MARKER_OBJ")
	fs := spec.Maps["FLOW_STATE"]
	if fs == nil {
		t.Fatal("no FLOW_STATE map")
	}
	if fs.Type != ciliumebpf.LRUHash || fs.MaxEntries != 65536 {
		t.Errorf("FLOW_STATE is %s max %d, want LRUHash max 65536", fs.Type, fs.MaxEntries)
	}
	st := spec.Maps["STATS"]
	if st == nil || st.MaxEntries < 9 {
		t.Errorf("STATS = %+v, want at least 9 entries (key 8: failed FLOW_STATE inserts)", st)
	}
}

// D6 = A: flow-tracker writes no FLOW_EVENTS ring (nothing drained it; 95%
// of its events were dropped).
//
//	FLOW_TRACKER_OBJ=$PWD/ebpf/target/load-gate/default/bpfel-unknown-none/release/flow-tracker \
//	    go test ./cmd/trace-collector-go -run TestBuiltFlowTrackerHasNoEventsRing -count=1
func TestBuiltFlowTrackerHasNoEventsRing(t *testing.T) {
	spec := builtSpec(t, "FLOW_TRACKER_OBJ")
	if _, ok := spec.Maps["FLOW_EVENTS"]; ok {
		t.Error("flow-tracker still has a FLOW_EVENTS map")
	}
	if spec.Maps["FLOWS"] == nil {
		t.Error("flow-tracker has no FLOWS map: the flow reader would have nothing to read")
	}
}
