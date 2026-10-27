// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package encoding_test

import (
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"unheaded/pkg/ebpf"
	"unheaded/pkg/protocol/encoding"
	"unheaded/pkg/protocol/integrity"
)

// repoRoot is the module root relative to this package directory.
const repoRoot = "../../.."

// crc16Reference is an independent table-driven CRC-16/CCITT-FALSE
// (poly 0x1021, init 0xFFFF, no reflection, no final XOR). It exists only
// to cross-check encoding.CRC16CCITT; production code must not copy it.
func crc16Reference(data []byte) uint16 {
	var table [256]uint16
	for i := range table {
		c := uint16(i) << 8
		for j := 0; j < 8; j++ {
			if c&0x8000 != 0 {
				c = c<<1 ^ 0x1021
			} else {
				c <<= 1
			}
		}
		table[i] = c
	}
	crc := uint16(0xFFFF)
	for _, b := range data {
		crc = crc<<8 ^ table[byte(crc>>8)^b]
	}
	return crc
}

// TestCRC16SingleImplementation enforces that the Monad checksum has one
// implementation, encoding.CRC16CCITT (S-TRUE-WIRE Step 108). Any other
// non-test Go file that defines a CRC16CCITT function or carries the 0x1021
// polynomial in code is a second copy.
func TestCRC16SingleImplementation(t *testing.T) {
	defRe := regexp.MustCompile(`(?m)^func\s+(\([^)]*\)\s*)?(CRC16CCITT|crc16CCITT|ComputeCRC16CCITT)\(`)
	polyRe := regexp.MustCompile(`0x1021\b`)
	canonical := filepath.Join("pkg", "protocol", "encoding", "encoding.go")

	var offenders []string
	for _, top := range []string{"pkg", "cmd", "services", "internal"} {
		dir := filepath.Join(repoRoot, top)
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if name := d.Name(); name == "target" || name == "vendor" || name == "node_modules" || name == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, _ := filepath.Rel(repoRoot, path)
			if rel == canonical {
				return nil
			}
			src, err := os.ReadFile(path) // #nosec G304 -- walking the repo's own source tree
			if err != nil {
				return err
			}
			if defRe.Match(src) {
				offenders = append(offenders, rel+": defines a CRC16CCITT function")
			}
			for i, line := range strings.Split(string(src), "\n") {
				code := line
				if k := strings.Index(code, "//"); k >= 0 {
					code = code[:k]
				}
				if polyRe.MatchString(code) {
					offenders = append(offenders, rel+":"+strconv.Itoa(i+1)+": carries the CRC-16 polynomial 0x1021")
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	for _, o := range offenders {
		t.Errorf("second CRC-16/CCITT implementation: %s (call encoding.CRC16CCITT)", o)
	}
}

// TestCRC16GoldenVectors pins the standard check value and the vectors the
// Monad wire tests rely on.
func TestCRC16GoldenVectors(t *testing.T) {
	cases := []struct {
		in   []byte
		want uint16
	}{
		{[]byte("123456789"), 0x29B1},
		{nil, 0xFFFF},
		{[]byte{}, 0xFFFF},
		{[]byte{0x00}, 0xE1F0},
	}
	for _, c := range cases {
		if got := encoding.CRC16CCITT(c.in); got != c.want {
			t.Errorf("CRC16CCITT(%q) = 0x%04X, want 0x%04X", c.in, got, c.want)
		}
	}
}

// TestCRC16MatchesRustDocVector reads the check value from the Rust
// CrcCcitt doc test in ebpf/monad-common (what the verifier loads) and
// asserts Go computes the same value.
func TestCRC16MatchesRustDocVector(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(repoRoot, "ebpf", "monad-common", "src", "lib.rs"))
	if err != nil {
		t.Skipf("Rust source not present: %v", err)
	}
	re := regexp.MustCompile(`CrcCcitt::compute\(b"([^"]*)"\);\s*\n\s*///\s*assert_eq!\(crc,\s*0x([0-9A-Fa-f]{4})\)`)
	m := re.FindSubmatch(src)
	if m == nil {
		t.Fatal("CrcCcitt doc-test vector not found in ebpf/monad-common/src/lib.rs; the parser must not silently pass")
	}
	want, _ := strconv.ParseUint(string(m[2]), 16, 16)
	if got := encoding.CRC16CCITT(m[1]); got != uint16(want) {
		t.Errorf("Go CRC16CCITT(%q) = 0x%04X, Rust doc test says 0x%04X", m[1], got, want)
	}
}

// TestCRC16EntryPointsAgree feeds 1,000 seeded random buffers through every
// exported entry point that computes the Monad checksum and an independent
// reference; all must agree.
func TestCRC16EntryPointsAgree(t *testing.T) {
	rng := rand.New(rand.NewSource(20261027)) // #nosec G404 -- deterministic test corpus
	for i := 0; i < 1000; i++ {
		buf := make([]byte, rng.Intn(64))
		rng.Read(buf)
		want := crc16Reference(buf)
		if got := encoding.CRC16CCITT(buf); got != want {
			t.Fatalf("buf %d: encoding.CRC16CCITT = 0x%04X, reference 0x%04X", i, got, want)
		}
		if !integrity.VerifyCRC16CCITT(buf, want) {
			t.Fatalf("buf %d: integrity.VerifyCRC16CCITT rejected the reference CRC 0x%04X", i, want)
		}

		var raw [ebpf.MonadSize]byte
		rng.Read(raw[:])
		m, err := ebpf.DecodeMonadState(raw[:])
		if err != nil {
			t.Fatalf("buf %d: DecodeMonadState: %v", i, err)
		}
		m.RecomputeChecksum()
		enc := m.Encode()
		if want := crc16Reference(enc[:ebpf.MonadCRCDataLen]); m.Checksum != want {
			t.Fatalf("buf %d: MonadState.RecomputeChecksum = 0x%04X, reference 0x%04X", i, m.Checksum, want)
		}
		if !m.VerifyChecksum() {
			t.Fatalf("buf %d: MonadState.VerifyChecksum rejected its own checksum", i)
		}
	}
}
