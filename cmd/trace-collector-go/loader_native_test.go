// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package main

import (
	"errors"
	"testing"
)

// The latency-probe object holds six kprobe functions; pkg/ebpf's loader
// submits one program per object, so the native path loaded one of them,
// attached none (every per-function Attach was "program not found"), and
// main still reported latency_probe as loaded. LoadLatencyKprobes
// (cilium/ebpf) is the only path that can load it.
func TestNativeBPFLoader_RefusesKprobeObject(t *testing.T) {
	n := &NativeBPFLoader{programs: make(map[string]*nativeProgram)}
	for _, p := range []string{"/x/latency-probe", "/x/latency_probe"} {
		if err := n.Load(p); !errors.Is(err, errKprobeObjectUnsupported) {
			t.Errorf("Load(%q) = %v, want errKprobeObjectUnsupported", p, err)
		}
	}
	if len(n.programs) != 0 {
		t.Errorf("refused load registered %d programs", len(n.programs))
	}
}
