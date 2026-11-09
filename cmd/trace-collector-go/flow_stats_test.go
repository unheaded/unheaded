// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"flag"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Every STAT_* key flow-tracker writes into FLOW_STATS comes out under its
// own name, lower-cased, and nothing else does: the names are read from
// the Rust source, so a renumbered or added counter fails here.
func TestCollectFlowStats_NamesMatchFlowTrackerKeys(t *testing.T) {
	src, err := os.ReadFile("../../ebpf/flow-tracker/src/main.rs")
	if err != nil {
		t.Fatal(err)
	}
	consts := regexp.MustCompile(`(?m)^const STAT_([A-Z_]+): u32 = (\d+);`).FindAllStringSubmatch(string(src), -1)
	if len(consts) < 10 {
		t.Fatalf("found %d STAT_ constants in flow-tracker; the pattern no longer matches", len(consts))
	}

	m := NewMockMapReader()
	want := map[string]uint64{}
	for _, c := range consts {
		k, _ := strconv.Atoi(c[2])
		key := make([]byte, 4)
		binary.LittleEndian.PutUint32(key, uint32(k))
		val := make([]byte, 8)
		binary.LittleEndian.PutUint64(val, uint64(1000+k))
		m.Insert(key, val)
		want[strings.ToLower(c[1])] = uint64(1000 + k)
	}
	// A key flow-tracker does not define, and a short value, are ignored.
	m.Insert([]byte{9, 0, 0, 0}, []byte{1, 0, 0, 0, 0, 0, 0, 0})
	m.Insert([]byte{0, 1, 0, 0}, []byte{7})

	stats, err := collectFlowStats(m)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(stats)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]uint64
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	for name, v := range want {
		if got[name] != v {
			t.Errorf("%s = %d, want %d (FLOW_STATS key %d)", name, got[name], v, v-1000)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("published %q, which flow-tracker does not define", name)
		}
	}
}

// recordingStatsSink keeps every FLOW_STATS snapshot handed to it.
type recordingStatsSink struct {
	got chan FlowStats
}

func (s *recordingStatsSink) PublishStats(payload json.RawMessage) {
	var fs FlowStats
	if err := json.Unmarshal(payload, &fs); err != nil {
		panic(err)
	}
	s.got <- fs
}

// noStatsMap is a stats source whose FLOW_STATS map is not there
// (flow_tracker not attached).
type noStatsMap struct{}

func (noStatsMap) GetStatsMap() MapReader { return nil }

func putStat(m *MockMapReader, key uint32, val uint64) {
	k := make([]byte, 4)
	binary.LittleEndian.PutUint32(k, key)
	v := make([]byte, 8)
	binary.LittleEndian.PutUint64(v, val)
	m.Insert(k, v)
}

// traces.stats goes out on every tick, whatever the trace reader does: it
// used to ride the poll path, so in ringbuf mode (the normal one) it never
// went out at all. Each tick reads the counters afresh.
func TestRunFlowStats_PublishesEachTick(t *testing.T) {
	loader := NewMockBPFLoader()
	putStat(loader.statsMap, 0, 10) // STAT_PACKETS_INGRESS
	putStat(loader.statsMap, 2, 3)  // STAT_FLOWS_NEW

	sink := &recordingStatsSink{got: make(chan FlowStats, 4)}
	tick := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runFlowStats(ctx, loader, sink, tick)
		close(done)
	}()

	tick <- time.Now()
	first := <-sink.got
	if first.PacketsIngress != 10 || first.FlowsNew != 3 {
		t.Errorf("first snapshot = %+v, want packets_ingress 10, flows_new 3", first)
	}

	putStat(loader.statsMap, 0, 25)
	tick <- time.Now()
	second := <-sink.got
	if second.PacketsIngress != 25 {
		t.Errorf("second snapshot packets_ingress = %d, want 25", second.PacketsIngress)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runFlowStats did not stop when its context ended")
	}
	if n := len(sink.got); n != 0 {
		t.Errorf("%d snapshots published without a tick", n)
	}
}

// No FLOW_STATS map: nothing published, and the loop keeps going.
func TestRunFlowStats_NoMapPublishesNothing(t *testing.T) {
	sink := &recordingStatsSink{got: make(chan FlowStats, 4)}
	tick := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runFlowStats(ctx, noStatsMap{}, sink, tick)
		close(done)
	}()
	tick <- time.Now()
	tick <- time.Now() // received: the first tick did not end the loop
	cancel()
	<-done
	if n := len(sink.got); n != 0 {
		t.Errorf("published %d snapshots with no FLOW_STATS map", n)
	}
}

// The stats interval is a flag with a sane default; poll mode's
// --read-interval went with poll mode.
func TestStatsIntervalFlag(t *testing.T) {
	f := flag.Lookup("stats-interval")
	if f == nil {
		t.Fatal("no --stats-interval flag")
	}
	if f.DefValue != "10s" {
		t.Errorf("--stats-interval default = %s, want 10s", f.DefValue)
	}
	if flag.Lookup("read-interval") != nil {
		t.Error("--read-interval still defined; it only drove the deleted poll path")
	}
}
