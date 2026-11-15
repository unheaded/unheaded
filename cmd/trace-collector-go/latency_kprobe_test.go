// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build linux

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ciliumebpf "github.com/cilium/ebpf"

	"unheaded/pkg/ebpf/objtrust"
)

// L17: the kprobe path takes the same check as the native loader, and the same
// opt-out, before it opens the object.
func TestLoadLatencyKprobes_ChecksTheObject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "latency-probe")
	if err := os.WriteFile(path, []byte("not an object"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadLatencyKprobes(path, false); !errors.Is(err, objtrust.ErrUntrusted) {
		t.Fatalf("err = %v, want objtrust.ErrUntrusted", err)
	}
	if _, _, err := LoadLatencyKprobes(path, true); err == nil || errors.Is(err, objtrust.ErrUntrusted) {
		t.Fatalf("with the opt-out: err = %v, want a parse failure", err)
	}
}

// The collector reads LATENCY_MAP as an LRU hash with a 24-byte key and a
// 40-byte value. Any other layout is refused before the object is loaded:
// west ran a 2026-10-18 object (40-byte key) under a 2026-11-10 binary, every
// entry failed to decode, and nothing said so (FND-044).
func TestCheckLatencyMapSpec(t *testing.T) {
	cases := []struct {
		name string
		spec *ciliumebpf.MapSpec
		want string // substring of the error; "" means no error
	}{
		{"the layout the collector reads", &ciliumebpf.MapSpec{Type: ciliumebpf.LRUHash, KeySize: 24, ValueSize: 40}, ""},
		{"a plain hash fills up and drops new threads", &ciliumebpf.MapSpec{Type: ciliumebpf.Hash, KeySize: 24, ValueSize: 40}, "LRU"},
		{"west's object of 2026-10-18", &ciliumebpf.MapSpec{Type: ciliumebpf.LRUHash, KeySize: 40, ValueSize: 40}, "key 40 bytes, want 24"},
		{"a short value", &ciliumebpf.MapSpec{Type: ciliumebpf.LRUHash, KeySize: 24, ValueSize: 32}, "value 32 bytes, want 40"},
		{"no LATENCY_MAP in the object", nil, "no LATENCY_MAP"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkLatencyMapSpec(tc.spec)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one naming %q", err, tc.want)
			}
		})
	}
}

// TestBuiltLatencyProbeSpec checks a built latency-probe object against the
// layout the collector reads. It needs no root: it parses the ELF only.
//
//	LATENCY_PROBE_OBJ=$PWD/ebpf/target/load-gate/default/bpfel-unknown-none/release/latency-probe \
//	    go test ./cmd/trace-collector-go -run TestBuiltLatencyProbeSpec -count=1
//
// A set variable naming a missing or unreadable file fails: this never passes
// by skipping once it was asked to check an object.
func TestBuiltLatencyProbeSpec(t *testing.T) {
	path := os.Getenv("LATENCY_PROBE_OBJ")
	if path == "" {
		t.Skip("set LATENCY_PROBE_OBJ to a built latency-probe object to check its LATENCY_MAP layout")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("LATENCY_PROBE_OBJ: %v", err)
	}
	defer f.Close()
	spec, err := ciliumebpf.LoadCollectionSpecFromReader(f)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if err := checkLatencyMapSpec(spec.Maps["LATENCY_MAP"]); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	ms := spec.Maps["LATENCY_MAP"]
	t.Logf("%s: LATENCY_MAP %s key %d value %d max %d", path, ms.Type, ms.KeySize, ms.ValueSize, ms.MaxEntries)
}
