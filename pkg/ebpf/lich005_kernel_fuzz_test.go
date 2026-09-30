// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux && lich005kernel

// LICH-005, kernel half: fuzzed .bpf.o objects through the real loader into
// bpf(2). The userspace half (lich005_fuzz_test.go) stops at our parsers;
// this one follows every object they accept into BPF_MAP_CREATE,
// BPF_MAP_UPDATE_ELEM (.rodata), BPF_PROG_LOAD and the verifier, then
// unloads. Nothing is ever attached or pinned.
//
// It needs root and it can panic the kernel, so it only builds with
// -tags lich005kernel and only runs with LICH005_KERNEL=1, on the
// sacrificial host (east, ADR-062). Run it inside a memory-capped cgroup:
// fuzzed map definitions ask for arbitrary sizes, and BPF map memory is
// charged to the caller's memcg.
//
//	sudo systemd-run --scope -p MemoryMax=2G env LICH005_KERNEL=1 \
//	  go test -tags lich005kernel ./pkg/ebpf -run '^$' \
//	  -fuzz '^FuzzKernelLoad$' -fuzztime 30m -fuzzminimizetime 2s -parallel 2
//
// Per input it checks: no panic (the fuzz engine reports one), Load either
// succeeds or returns an error, a successful Load unloads cleanly, and the
// process holds exactly as many fds afterwards as before, so an error path
// that forgets a map or program fd fails here. Kernel-side warnings are
// checked from dmesg by the campaign script, not per input.

package ebpf

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
)

var kernelLoadSeq atomic.Uint64

func openFDs(t *testing.T) int {
	t.Helper()
	ents, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatalf("read /proc/self/fd: %v", err)
	}
	return len(ents)
}

func FuzzKernelLoad(f *testing.F) {
	if os.Getenv("LICH005_KERNEL") != "1" || os.Geteuid() != 0 {
		f.Skip("kernel half: needs root and LICH005_KERNEL=1, on a sacrificial host only")
	}
	for _, b := range append(realBPFObjects(f, 5), btfBPFObjects()...) {
		f.Add(b)
	}
	f.Add([]byte("\x7fELF"))

	cfg := DefaultLoaderConfig()
	cfg.PinPath = "" // never pin
	cfg.MetricsEnabled = false
	cfg.VerifierLogSize = 16 * 1024
	// Inputs are written to the (world-writable) temp dir; this harness
	// fuzzes object bytes, not path trust (objtrust_test.go covers that).
	cfg.AllowUntrustedObjectPaths = true
	l, err := NewNativeLoader(cfg)
	if err != nil {
		f.Fatalf("NewNativeLoader: %v", err)
	}
	dir := f.TempDir()

	f.Fuzz(func(t *testing.T, b []byte) {
		path := filepath.Join(dir, "in.bpf.o")
		if err := os.WriteFile(path, b, 0o600); err != nil {
			t.Fatal(err)
		}
		name := "lich005-" + strconv.FormatUint(kernelLoadSeq.Add(1), 10)

		before := openFDs(t)
		err := l.Load(context.Background(), &ProgramSpec{Name: name, Path: path})
		if err == nil {
			if uerr := l.Unload(context.Background(), name); uerr != nil {
				t.Fatalf("loaded %d-byte object, then Unload: %v", len(b), uerr)
			}
		}
		if after := openFDs(t); after != before {
			t.Fatalf("fd leak: %d open before Load, %d after (Load err: %v)", before, after, err)
		}
	})
}
