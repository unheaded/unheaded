// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package ebpf

import (
	"bytes"
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// LICH-005 kernel half, east, 2026-09-30: a crafted object panicked Load
// with "slice bounds out of range [:-72057594037927584]". A .text
// subprogram offset comes from an ELF symbol value (uint64); int(off)
// wrapped negative, passed `int(off)+size > len`, and the slice panicked.
func TestAppendSubprogram_Bounds(t *testing.T) {
	text := bytes.Repeat([]byte{0xAA}, 64)
	for _, tc := range []struct {
		name string
		off  uint64
		size int
		ok   bool
	}{
		{"in range", 8, 16, true},
		{"exactly to the end", 48, 16, true},
		{"past the end", 56, 16, false},
		{"zero size", 8, 0, false},
		{"negative size", 8, -8, false},
		{"offset wraps int", 1 << 63, 16, false},
		{"offset near max", math.MaxUint64 - 7, 16, false},
		{"fuzz-found offset", 0xff00000000000130, 64, false},
		{"size overflows offset", 8, math.MaxInt, false},
	} {
		dst := []byte{1}
		got, ok := appendSubprogram(dst, text, tc.off, tc.size)
		if ok != tc.ok {
			t.Errorf("%s: ok = %v, want %v", tc.name, ok, tc.ok)
		}
		if want := 1 + tc.size; ok && len(got) != want {
			t.Errorf("%s: len = %d, want %d", tc.name, len(got), want)
		}
		if !ok && len(got) != 1 {
			t.Errorf("%s: dst changed on refusal", tc.name)
		}
	}
}

func TestFindFuncSize_OffsetPastText(t *testing.T) {
	p := &parsedELF{
		FuncSizes: map[uint64]int{},
		Programs:  map[string]*elfProgram{".text": {Instructions: make([]byte, 32)}},
	}
	for _, off := range []uint64{32, 1 << 63, math.MaxUint64} {
		if got := findFuncSize(p, off); got != 0 {
			t.Errorf("findFuncSize(%#x) = %d, want 0", off, got)
		}
	}
}

// Load chose its main program by ranging over a Go map, and an unspecified
// program type (0) matched .text and .rodata* sections, so the same object
// loaded a data section as code on some runs and panicked on others.
func TestSelectMainProgram(t *testing.T) {
	p := &parsedELF{Programs: map[string]*elfProgram{
		".text":        {Name: ".text"},
		".rodata.cst8": {Name: ".rodata.cst8"},
		"xdp/b":        {Name: "xdp/b", Type: BPF_PROG_TYPE_XDP},
		"xdp/a":        {Name: "xdp/a", Type: BPF_PROG_TYPE_XDP},
		"kprobe/x":     {Name: "kprobe/x", Type: BPF_PROG_TYPE_KPROBE},
	}}
	for i := 0; i < 50; i++ {
		if got := selectMainProgram(p, BPF_PROG_TYPE_UNSPEC); got == nil || got.Name != "kprobe/x" {
			t.Fatalf("unspecified type: got %v, want kprobe/x (first entry section by name)", got)
		}
		if got := selectMainProgram(p, BPF_PROG_TYPE_XDP); got == nil || got.Name != "xdp/a" {
			t.Fatalf("XDP: got %v, want xdp/a", got)
		}
	}
	if got := selectMainProgram(p, BPF_PROG_TYPE_SCHED_CLS); got == nil || got.Name != "kprobe/x" {
		t.Fatalf("no type match: got %v, want the first entry section", got)
	}
	onlyData := &parsedELF{Programs: map[string]*elfProgram{".text": {Name: ".text"}, ".rodata": {Name: ".rodata"}}}
	if got := selectMainProgram(onlyData, BPF_PROG_TYPE_UNSPEC); got != nil {
		t.Fatalf("object with no entry section: got %q, want nil", got.Name)
	}
}

// The fuzz-found object itself, through Load, unprivileged: before the fix
// ~1 run in 10 panicked (map order). It must now fail the same way every
// time, without panicking.
func TestLoad_FuzzFoundSubprogramOffset(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "fuzz", "FuzzKernelLoad", "7fbfa6d6a9e2645f"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "crash.bpf.o")
	if err := os.WriteFile(path, corpusBytes(t, raw), 0o600); err != nil {
		t.Fatal(err)
	}
	var first string
	for i := 0; i < 40; i++ {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("run %d: Load panicked: %v", i, r)
				}
			}()
			l := &NativeLoader{config: DefaultLoaderConfig(), programs: make(map[string]*loadedProgram)}
			err := l.Load(context.Background(), &ProgramSpec{Name: "crash", Path: path})
			if err == nil {
				_ = l.Unload(context.Background(), "crash")
				return
			}
			if first == "" {
				first = err.Error()
			} else if err.Error() != first {
				t.Fatalf("run %d: %q, but run 0: %q (nondeterministic)", i, err, first)
			}
		}()
	}
}
