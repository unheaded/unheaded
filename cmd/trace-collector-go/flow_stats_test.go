// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
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
