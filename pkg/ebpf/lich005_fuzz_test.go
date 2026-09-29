// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// LICH-005, userspace half: our own parsers on the kernel boundary. The
// loader parses .bpf.o ELF and BTF by hand before anything reaches bpf(2),
// and the decoders read bytes BPF programs wrote into maps and ring
// buffers. Any input must yield a value or an error, never a panic.
//
// Seeds include the real compiled programs when ebpf/ has been built
// (scripts/check-ebpf-loads.sh builds them into target/load-gate).
//
//	go test ./pkg/ebpf -run '^$' -fuzz FuzzParseELF -fuzztime 60s

package ebpf

import (
	"bytes"
	"debug/elf"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// realBPFObjects returns the compiled BPF programs, smallest first, or none.
func realBPFObjects(t testing.TB, max int) [][]byte {
	dir := filepath.Join("..", "..", "ebpf", "target", "load-gate", "default", "bpfel-unknown-none", "release")
	var out [][]byte
	for _, name := range []string{"xdp-redirect", "compliance-ebpf", "syscall-tracer", "packet-marker", "failover-ebpf"} {
		if len(out) == max {
			break
		}
		if b, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			out = append(out, b)
		}
	}
	return out
}

func FuzzParseELF(f *testing.F) {
	for _, b := range realBPFObjects(f, 3) {
		f.Add(b)
	}
	f.Add([]byte("\x7fELF"))
	// testdata/fuzz/FuzzParseELF holds the input that panicked debug/elf.
	f.Fuzz(func(t *testing.T, b []byte) {
		p, err := parseELFBytes(b)
		if (p == nil) == (err == nil) {
			t.Fatalf("parseELFBytes = %v, %v: want exactly one of result or error", p, err)
		}
	})
}

func FuzzParseBTF(f *testing.F) {
	for _, obj := range realBPFObjects(f, 5) {
		ef, err := elf.NewFile(bytes.NewReader(obj))
		if err != nil {
			continue
		}
		if s := ef.Section(".BTF"); s != nil {
			if d, err := s.Data(); err == nil {
				f.Add(d)
			}
		}
	}
	hdr := make([]byte, 24)
	hdr[0], hdr[1] = 0x9f, 0xeb // BTF magic 0xeB9F, little-endian
	f.Add(hdr)
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = parseBTF(b) })
}

func FuzzParseMapsSection(f *testing.F) {
	f.Add(make([]byte, 28))
	f.Add(make([]byte, 20))
	f.Add(make([]byte, 56))
	f.Fuzz(func(t *testing.T, b []byte) {
		r := &parsedELF{Maps: map[string]*elfMap{}}
		if n := parseMapsSection(b, r); n != 20 && n != 28 {
			t.Fatalf("def size %d", n)
		}
	})
}

func FuzzDecodeMonadState(f *testing.F) {
	f.Add(make([]byte, MonadSize))
	f.Fuzz(func(t *testing.T, b []byte) {
		v, err := DecodeMonadState(b)
		if err != nil {
			return
		}
		enc := v.Encode()
		v2, err := DecodeMonadState(enc[:])
		if err != nil || !reflect.DeepEqual(v, v2) {
			t.Fatalf("round trip: %+v -> %+v (%v)", v, v2, err)
		}
	})
}

func FuzzDecodeAnamnesisEvent(f *testing.F) {
	f.Add(make([]byte, AnamnesisEventSize))
	ok := make([]byte, AnamnesisEventSize)
	ok[8] = 1
	f.Add(ok)
	f.Fuzz(func(t *testing.T, b []byte) {
		v, err := DecodeAnamnesisEvent(b)
		if err != nil {
			return
		}
		enc := v.Encode()
		v2, err := DecodeAnamnesisEvent(enc[:])
		if err != nil || !reflect.DeepEqual(v, v2) {
			t.Fatalf("round trip: %+v -> %+v (%v)", v, v2, err)
		}
	})
}

// Replaces an exported FuzzDecodeComputeHopEvent(data []byte) in
// anamnesis.go that no fuzzer ever ran (its "fuzz_test.go" did not exist);
// its table cases are the seeds here.
func FuzzDecodeComputeHopEvent(f *testing.F) {
	for _, n := range []int{0, 1, 93, 94, 95, 200} {
		f.Add(make([]byte, n))
	}
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = DecodeComputeHopEvent(b) })
}

// The production entry point, from a file: the object that panicked
// debug/elf must come back as ErrELFParseFailed.
func TestParseELF_MalformedObjectIsAnError(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "fuzz", "FuzzParseELF", "c8f58c9be41661ef"))
	if err != nil {
		t.Fatal(err)
	}
	// The corpus file is "go test fuzz v1" text; the object is its []byte literal.
	b := corpusBytes(t, raw)
	path := filepath.Join(t.TempDir(), "evil.bpf.o")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := parseELF(path)
	if p != nil || !errors.Is(err, ErrELFParseFailed) {
		t.Fatalf("parseELF = %v, %v; want ErrELFParseFailed", p, err)
	}
}

// corpusBytes decodes a one-value "go test fuzz v1" []byte corpus file.
func corpusBytes(t *testing.T, raw []byte) []byte {
	t.Helper()
	s := string(raw)
	i := strings.Index(s, "[]byte(")
	j := strings.LastIndex(s, ")")
	if i < 0 || j < i {
		t.Fatalf("not a []byte corpus file")
	}
	v, err := strconv.Unquote(s[i+len("[]byte(") : j])
	if err != nil {
		t.Fatal(err)
	}
	return []byte(v)
}
