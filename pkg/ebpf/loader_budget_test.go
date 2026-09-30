// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package ebpf

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// buildBPFObject hand-assembles a minimal EM_BPF relocatable object: an
// "xdp" program (r0 = 0; exit), a GPL license and a "maps" section holding
// one 28-byte Aya-style definition per entry of defs
// ({type, key_size, value_size, max_entries, flags}).
func buildBPFObject(defs [][5]uint32) []byte {
	le := binary.LittleEndian
	shstr := []byte("\x00.shstrtab\x00maps\x00xdp\x00license\x00")
	nameOff := map[string]uint32{".shstrtab": 1, "maps": 11, "xdp": 16, "license": 20}

	maps := make([]byte, 28*len(defs))
	for i, d := range defs {
		for j, v := range d {
			le.PutUint32(maps[28*i+4*j:], v)
		}
	}
	prog := []byte{0xb7, 0, 0, 0, 0, 0, 0, 0, 0x95, 0, 0, 0, 0, 0, 0, 0}
	license := []byte("GPL\x00")

	type sec struct {
		name  string
		typ   uint32
		flags uint64
		data  []byte
		align uint64
		off   uint64
	}
	secs := []*sec{
		{name: ".shstrtab", typ: 3, data: shstr, align: 1},
		{name: "maps", typ: 1, flags: 3, data: maps, align: 4},
		{name: "xdp", typ: 1, flags: 6, data: prog, align: 8},
		{name: "license", typ: 1, flags: 3, data: license, align: 1},
	}

	out := make([]byte, 64)
	for _, s := range secs {
		for uint64(len(out))%8 != 0 {
			out = append(out, 0)
		}
		s.off = uint64(len(out))
		out = append(out, s.data...)
	}
	for len(out)%8 != 0 {
		out = append(out, 0)
	}
	shoff := uint64(len(out))
	out = append(out, make([]byte, 64)...) // SHN_UNDEF
	for _, s := range secs {
		h := make([]byte, 64)
		le.PutUint32(h[0:], nameOff[s.name])
		le.PutUint32(h[4:], s.typ)
		le.PutUint64(h[8:], s.flags)
		le.PutUint64(h[24:], s.off)
		le.PutUint64(h[32:], uint64(len(s.data)))
		le.PutUint64(h[48:], s.align)
		out = append(out, h...)
	}

	copy(out[0:], []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	le.PutUint16(out[16:], 1)   // ET_REL
	le.PutUint16(out[18:], 247) // EM_BPF
	le.PutUint32(out[20:], 1)
	le.PutUint64(out[40:], shoff)
	le.PutUint16(out[52:], 64)
	le.PutUint16(out[58:], 64)
	le.PutUint16(out[60:], uint16(len(secs)+1))
	le.PutUint16(out[62:], 1) // .shstrtab
	return out
}

func TestBuildBPFObject_Parses(t *testing.T) {
	p, err := parseELFBytes(buildBPFObject([][5]uint32{{BPF_MAP_TYPE_ARRAY, 4, 8, 16, 0}}))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Programs) != 1 || len(p.Maps) != 1 || p.License != "GPL" {
		t.Fatalf("parsed %d programs, %d maps, license %q", len(p.Programs), len(p.Maps), p.License)
	}
}

func TestMapBytes(t *testing.T) {
	for _, tc := range []struct {
		name string
		m    elfMap
		want uint64
	}{
		{"array", elfMap{Type: BPF_MAP_TYPE_ARRAY, KeySize: 4, ValueSize: 4096, MaxEntries: 1 << 19}, 4100 << 19},
		{"ringbuf is max_entries bytes", elfMap{Type: BPF_MAP_TYPE_RINGBUF, MaxEntries: 1 << 24}, 1 << 24},
		{"product saturates", elfMap{Type: BPF_MAP_TYPE_HASH, KeySize: math.MaxUint32, ValueSize: math.MaxUint32, MaxEntries: math.MaxUint32}, math.MaxUint64},
	} {
		if got := mapBytes(&tc.m); got != tc.want {
			t.Errorf("%s: mapBytes = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestObjectMapBytes_SumSaturates(t *testing.T) {
	big := &elfMap{Type: BPF_MAP_TYPE_HASH, KeySize: math.MaxUint32, ValueSize: math.MaxUint32, MaxEntries: math.MaxUint32}
	p := &parsedELF{Maps: map[string]*elfMap{"a": big, "b": big}, Programs: map[string]*elfProgram{}}
	if got := objectMapBytes(p); got != math.MaxUint64 {
		t.Fatalf("objectMapBytes = %d, want saturation at MaxUint64", got)
	}
}

// LICH-005 kernel half: one crafted object took ~2.4 GB of kernel memory on
// east, allocated by BPF_MAP_CREATE before the verifier saw the program.
// The budget refuses it before any bpf(2) call, so this runs unprivileged:
// without the check, Load would reach the syscall and fail differently.
func TestLoad_MapBudgetRefusedBeforeSyscalls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.bpf.o")
	obj := buildBPFObject([][5]uint32{{BPF_MAP_TYPE_ARRAY, 4, 4096, 1 << 19, 0}})
	if err := os.WriteFile(path, obj, 0o600); err != nil {
		t.Fatal(err)
	}

	for _, cfg := range []LoaderConfig{
		DefaultLoaderConfig(),
		{}, // zero value: MaxMapBytes 0 means the default, not unlimited
	} {
		l := &NativeLoader{config: cfg, programs: make(map[string]*loadedProgram)}
		err := l.Load(context.Background(), &ProgramSpec{Name: "big", Path: path})
		if !errors.Is(err, ErrMapBudgetExceeded) {
			t.Fatalf("MaxMapBytes=%d: Load = %v, want ErrMapBudgetExceeded", cfg.MaxMapBytes, err)
		}
	}
}

func TestLoad_WithinBudgetPassesTheCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "small.bpf.o")
	if err := os.WriteFile(path, buildBPFObject([][5]uint32{{BPF_MAP_TYPE_ARRAY, 4, 8, 16, 0}}), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultLoaderConfig()
	l := &NativeLoader{config: cfg, programs: make(map[string]*loadedProgram)}
	err := l.Load(context.Background(), &ProgramSpec{Name: "small", Path: path})
	if errors.Is(err, ErrMapBudgetExceeded) {
		t.Fatalf("a 192-byte map was refused: %v", err)
	}
	if err == nil {
		_ = l.Unload(context.Background(), "small")
	}
}
