// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package wal

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// damagedSegment is one good entry, then a header claiming a body far larger
// than the file.
func damagedSegment(t *testing.T) *Segment {
	t.Helper()
	path := filepath.Join(t.TempDir(), "seg.wal")
	seg, err := CreateSegment(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := seg.Write(1, []byte("good")); err != nil {
		t.Fatal(err)
	}
	if err := seg.Sync(); err != nil {
		t.Fatal(err)
	}
	header := make([]byte, entryHeaderSize)
	binary.BigEndian.PutUint64(header[0:8], 2)
	binary.BigEndian.PutUint32(header[8:12], 0xFFFFFFFF)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0) // #nosec G304 -- test file under t.TempDir
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(header); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	seg.size += entryHeaderSize
	t.Cleanup(func() { _ = seg.Close() })
	return seg
}

// allocated is the bytes allocated while fn ran.
func allocated(fn func()) uint64 {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	fn()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// L10: a length read from the file is checked against the file before
// anything is allocated for it.
func TestSegment_ImpossibleLengthAllocatesNothing(t *testing.T) {
	const budget = 1 << 20
	for name, read := range map[string]func(*Segment) error{
		"Read": func(s *Segment) error { _, err := s.Read(2); return err },
		"Scan": func(s *Segment) error {
			return s.Scan(0, func(uint64, []byte) error { return nil })
		},
		"ReadAll": func(s *Segment) error { _, err := s.ReadAll(); return err },
	} {
		seg := damagedSegment(t)
		var err error
		if got := allocated(func() { err = read(seg) }); got > budget {
			t.Errorf("%s allocated %d bytes for a damaged entry", name, got)
		}
		if !errors.Is(err, ErrCorruptedEntry) {
			t.Errorf("%s = %v, want ErrCorruptedEntry", name, err)
		}
	}
}

func TestSegment_RepairDropsImpossibleEntry(t *testing.T) {
	seg := damagedSegment(t)
	var err error
	if got := allocated(func() { err = seg.Repair() }); got > 1<<20 {
		t.Errorf("Repair allocated %d bytes", got)
	}
	if err != nil {
		t.Fatalf("Repair: %v", err)
	}
	entries, err := seg.ReadAll()
	if err != nil || len(entries) != 1 || string(entries[0].Data) != "good" {
		t.Fatalf("after Repair: %v, %v", entries, err)
	}
}
