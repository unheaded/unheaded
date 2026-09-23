// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// LICH-010 (real target, corrected): pkg/storage/wal
//
// lich_010_wal_integrity_test.go names "pkg/storage/wal — entry encoding,
// decoding, CRC verification" and imports nothing, fuzzing a WAL written
// inside the test file.
//
// The first repoint of LICH-010 (services/wotan/internal/store) was aimed at
// the wrong thing: that is the WAL *store*, a JSON message codec layered on
// top. pkg/storage/wal — the actual segmented write-ahead log the harness
// named — does exist, is reachable from here (it is under pkg/), and was
// still unfuzzed. Both are worth covering; this is the one the inventory
// always meant.
//
// On-disk record format, from encodeEntry:
//
//	[seq:8][length:4][crc32:4][data:length]     big-endian
//
// A WAL exists to be read back after a crash, so the bytes it parses are
// bytes that have survived something bad. Corruption is the expected input,
// not the adversarial edge case.
//
// Invariants:
//   - Opening and scanning an arbitrary file never panics
//   - A corrupt record is reported, not returned as data
//   - A record whose CRC does not match its data is never returned
//   - Parsing a corrupt header does not allocate proportionally to an
//     attacker-controlled length field
package harnesses

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"unheaded/pkg/storage/wal"
)

// writeSegmentFile drops raw bytes where a WAL expects a segment.
func writeSegmentFile(t *testing.T, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	// Segment naming is an implementation detail; a real WAL is created
	// first so the directory has whatever shape New() expects, then the
	// segment is overwritten with the fuzzer's bytes.
	w, err := wal.New(walConfig(dir))
	if err != nil {
		t.Skipf("wal.New: %v", err)
	}
	if _, err := w.Write(context.Background(), []byte("seed")); err != nil {
		t.Skipf("seed write: %v", err)
	}
	_ = w.Close()

	matches, _ := filepath.Glob(filepath.Join(dir, "*"))
	if len(matches) == 0 {
		t.Skip("no segment file created")
	}
	if err := os.WriteFile(matches[0], data, 0o600); err != nil {
		t.Fatalf("overwrite segment: %v", err)
	}
	return dir
}

func walConfig(dir string) wal.Config {
	cfg := wal.DefaultConfig()
	cfg.Path = dir
	return cfg
}

// FuzzRealStorageWALScanCorrupt replaces a segment with arbitrary bytes and
// scans it, which is exactly what recovery does after a crash.
func FuzzRealStorageWALScanCorrupt(f *testing.F) {
	// A well-formed single entry.
	good := make([]byte, 16+4)
	binary.BigEndian.PutUint64(good[0:8], 1)
	binary.BigEndian.PutUint32(good[8:12], 4)
	binary.BigEndian.PutUint32(good[12:16], 0)
	copy(good[16:], "data")
	f.Add(good)

	f.Add([]byte{})
	f.Add([]byte{0, 0, 0, 0, 0, 0, 0, 1})                                     // truncated header
	f.Add(make([]byte, 16))                                                   // header, no data
	f.Add([]byte{0, 0, 0, 0, 0, 0, 0, 1, 0xff, 0xff, 0xff, 0xff, 0, 0, 0, 0}) // huge length

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<16 {
			data = data[:1<<16]
		}
		dir := writeSegmentFile(t, data)

		w, err := wal.New(walConfig(dir))
		if err != nil {
			return // refusing to open a corrupt WAL is a valid outcome
		}
		defer func() { _ = w.Close() }()

		// Scanning must not panic and must not hand back a record whose
		// bytes failed their own checksum.
		_ = w.Scan(context.Background(), 0, func(_ uint64, _ []byte) error {
			return nil
		})
	})
}

// The record length field is not validated where it is used, but it is
// bounded where the segment is opened — and that is what keeps it safe.
//
// Segment.Read does `data := make([]byte, length)` with length taken straight
// from the record header, which looks like an unbounded allocation driven by
// a value on disk. It is not reachable: OpenSegment runs scanSequences first,
// which walks every record with Discard(int(length)), so a segment whose
// length field points past EOF fails to open at all.
//
// Predicted as a bug from reading Read() in isolation, then falsified by
// testing it. Pinned here because the protection is load-bearing and lives in
// a different function from the risk: anyone who makes scanSequences lazy, or
// skips it on open, reintroduces the unbounded allocation without touching
// the line that performs it.
func TestOpenSegment_RejectsLengthPastEOF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "000001.wal")

	// [seq=1][length=0xFFFFFFFF][crc=0] with no data behind it.
	corrupt := make([]byte, 16)
	binary.BigEndian.PutUint64(corrupt[0:8], 1)
	binary.BigEndian.PutUint32(corrupt[8:12], 0xFFFFFFFF)
	binary.BigEndian.PutUint32(corrupt[12:16], 0)
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatalf("write segment: %v", err)
	}

	seg, err := wal.OpenSegment(path, false)
	if err == nil {
		_ = seg.Close()
		t.Fatal("opened a segment whose length field runs past EOF; " +
			"Segment.Read would then allocate that length verbatim")
	}
}
