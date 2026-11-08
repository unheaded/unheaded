// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux && integration

package ebpf

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestParseCompiledBPFPrograms checks the Go ELF parser against the
// Aya-built objects: each program it should find, by name and type. A
// multi-function section is split into one program per function, so
// flow-tracker's "classifier" section reads as its two functions.
//
// Run it against the objects scripts/check-ebpf-loads.sh builds:
//
//	EBPF_GO_LOAD_DIR=ebpf/target/load-gate/default/bpfel-unknown-none/release \
//	    go test -tags integration ./pkg/ebpf/ -run TestParseCompiledBPFPrograms
func TestParseCompiledBPFPrograms(t *testing.T) {
	if runtime.GOARCH != "arm64" && runtime.GOARCH != "amd64" {
		t.Skip("BPF programs compiled for specific arch")
	}

	ebpfDir := findEBPFDir(t)
	if ebpfDir == "" {
		t.Skip("eBPF build directory not found")
	}

	programs := []struct {
		object string
		want   map[string]uint32 // program name -> BPF_PROG_TYPE_*
	}{
		{"packet-marker", map[string]uint32{"xdp": BPF_PROG_TYPE_XDP}},
		{"flow-tracker", map[string]uint32{
			"flow_tracker_ingress": BPF_PROG_TYPE_SCHED_CLS,
			"flow_tracker_egress":  BPF_PROG_TYPE_SCHED_CLS,
		}},
		{"latency-probe", map[string]uint32{
			"tcp_connect_enter": BPF_PROG_TYPE_KPROBE,
			"tcp_connect_exit":  BPF_PROG_TYPE_KPROBE,
			"tcp_sendmsg_enter": BPF_PROG_TYPE_KPROBE,
			"tcp_sendmsg_exit":  BPF_PROG_TYPE_KPROBE,
			"tcp_recvmsg_enter": BPF_PROG_TYPE_KPROBE,
			"tcp_recvmsg_exit":  BPF_PROG_TYPE_KPROBE,
		}},
	}

	for _, prog := range programs {
		t.Run(prog.object, func(t *testing.T) {
			path := filepath.Join(ebpfDir, prog.object)
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("BPF object missing from %s: %v", ebpfDir, err)
			}

			parsed, err := parseELF(path)
			if err != nil {
				t.Fatalf("failed to parse ELF %s: %v", prog.object, err)
			}

			for name, typ := range prog.want {
				p, ok := parsed.Programs[name]
				if !ok {
					t.Errorf("program %q not found in %s (found: %v)", name, prog.object, programNames(parsed))
					continue
				}
				if p.Type != typ {
					t.Errorf("%s/%s: type %d, want %d", prog.object, name, p.Type, typ)
				}
				if len(p.Instructions) == 0 || len(p.Instructions)%8 != 0 {
					t.Errorf("%s/%s: %d instruction bytes, want a non-zero multiple of 8", prog.object, name, len(p.Instructions))
				}
			}

			if len(parsed.Maps) == 0 {
				t.Errorf("no maps found in %s", prog.object)
			}

			t.Logf("%s: %d programs, %d maps", prog.object, len(parsed.Programs), len(parsed.Maps))
		})
	}
}

// findEBPFDir prefers EBPF_GO_LOAD_DIR, which scripts/check-ebpf-loads.sh
// sets for the Go loader test, over an older build in the tree.
func findEBPFDir(t *testing.T) string {
	t.Helper()
	if dir := os.Getenv("EBPF_GO_LOAD_DIR"); dir != "" {
		return dir
	}
	candidates := []string{
		filepath.Join("..", "..", "ebpf", "target", "load-gate", "default", "bpfel-unknown-none", "release"),
		filepath.Join("..", "..", "ebpf", "target", "bpfel-unknown-none", "release"),
	}
	for _, dir := range candidates {
		if _, err := os.Stat(filepath.Join(dir, "flow-tracker")); err == nil {
			return dir
		}
	}
	return ""
}

func programNames(p *parsedELF) []string {
	names := make([]string, 0, len(p.Programs))
	for name := range p.Programs {
		names = append(names, name)
	}
	return names
}
