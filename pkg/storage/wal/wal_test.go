// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wal

import (
	"context"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"

	"unheaded/pkg/storage"
)

// newTestWAL creates a WAL in a temp directory with sensible defaults.
func newTestWAL(t *testing.T) *WAL {
	t.Helper()
	dir := t.TempDir()
	w, err := New(Config{
		Path:        dir,
		SegmentSize: 4096,
		SyncWrites:  false,
		MaxSegments: 5,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { w.Close() })
	return w
}

// ---------- WAL-level tests ----------

func TestNew_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub", "wal")
	w, err := New(Config{
		Path:        dir,
		SegmentSize: 4096,
		SyncWrites:  false,
		MaxSegments: 5,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer w.Close()

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected directory, got file")
	}
}

func TestNew_InitialState(t *testing.T) {
	w := newTestWAL(t)

	if seq := w.LastSequence(); seq != 0 {
		t.Errorf("initial sequence = %d, want 0", seq)
	}
	if n := w.Segments(); n != 1 {
		t.Errorf("initial segments = %d, want 1", n)
	}
}

func TestWriteAndRead(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()

	data := []byte("hello kingdom")
	seq, err := w.Write(ctx, data)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if seq != 1 {
		t.Errorf("first sequence = %d, want 1", seq)
	}

	got, err := w.Read(ctx, seq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("Read = %q, want %q", got, data)
	}
}

func TestWriteMultiple(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		seq, err := w.Write(ctx, []byte{byte(i)})
		if err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
		if seq != uint64(i+1) {
			t.Errorf("seq = %d, want %d", seq, i+1)
		}
	}

	if last := w.LastSequence(); last != 10 {
		t.Errorf("LastSequence = %d, want 10", last)
	}

	// Read back each entry.
	for i := 1; i <= 10; i++ {
		got, err := w.Read(ctx, uint64(i))
		if err != nil {
			t.Fatalf("Read seq %d: %v", i, err)
		}
		if len(got) != 1 || got[0] != byte(i-1) {
			t.Errorf("Read seq %d = %v, want [%d]", i, got, i-1)
		}
	}
}

func TestRead_NotFound(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()

	_, err := w.Read(ctx, 999)
	if err != ErrSequenceNotFound {
		t.Errorf("Read non-existent = %v, want ErrSequenceNotFound", err)
	}
}

func TestScan(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if _, err := w.Write(ctx, []byte{byte(i)}); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	var scanned []uint64
	err := w.Scan(ctx, 3, func(seq uint64, data []byte) error {
		scanned = append(scanned, seq)
		return nil
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// Sequences 3, 4, 5 should be returned.
	if len(scanned) != 3 {
		t.Fatalf("scanned %d entries, want 3", len(scanned))
	}
	for i, want := range []uint64{3, 4, 5} {
		if scanned[i] != want {
			t.Errorf("scanned[%d] = %d, want %d", i, scanned[i], want)
		}
	}
}

func TestScan_ContextCancellation(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if _, err := w.Write(ctx, []byte{byte(i)}); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel() // cancel immediately

	err := w.Scan(cancelCtx, 1, func(seq uint64, data []byte) error {
		return nil
	})
	if err != context.Canceled {
		t.Errorf("Scan with cancelled context = %v, want context.Canceled", err)
	}
}

func TestClose_RejectsOperations(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()

	w.Close()

	_, err := w.Write(ctx, []byte("data"))
	if err != storage.ErrClosed {
		t.Errorf("Write after close = %v, want ErrClosed", err)
	}

	_, err = w.Read(ctx, 1)
	if err != storage.ErrClosed {
		t.Errorf("Read after close = %v, want ErrClosed", err)
	}

	err = w.Scan(ctx, 0, func(seq uint64, data []byte) error { return nil })
	if err != storage.ErrClosed {
		t.Errorf("Scan after close = %v, want ErrClosed", err)
	}

	err = w.Truncate(ctx, 1)
	if err != storage.ErrClosed {
		t.Errorf("Truncate after close = %v, want ErrClosed", err)
	}

	err = w.Sync()
	if err != storage.ErrClosed {
		t.Errorf("Sync after close = %v, want ErrClosed", err)
	}
}

func TestClose_Idempotent(t *testing.T) {
	w := newTestWAL(t)
	if err := w.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestSync(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()

	if _, err := w.Write(ctx, []byte("sync-me")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}
}

func TestSize(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()

	before := w.Size()
	if _, err := w.Write(ctx, []byte("payload")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	after := w.Size()
	if after <= before {
		t.Errorf("Size did not increase after write: before=%d, after=%d", before, after)
	}
}

func TestSegmentRotation(t *testing.T) {
	dir := t.TempDir()
	w, err := New(Config{
		Path:        dir,
		SegmentSize: 64, // very small to force rotation
		SyncWrites:  false,
		MaxSegments: 100,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer w.Close()
	ctx := context.Background()

	// Write enough data to trigger at least one rotation.
	bigPayload := make([]byte, 40)
	for i := 0; i < 5; i++ {
		if _, err := w.Write(ctx, bigPayload); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}

	if segs := w.Segments(); segs < 2 {
		t.Errorf("segments = %d, want >= 2 (rotation should have occurred)", segs)
	}
}

func TestTruncate(t *testing.T) {
	dir := t.TempDir()
	w, err := New(Config{
		Path:        dir,
		SegmentSize: 64, // small to get multiple segments
		SyncWrites:  false,
		MaxSegments: 100,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer w.Close()
	ctx := context.Background()

	bigPayload := make([]byte, 40)
	for i := 0; i < 10; i++ {
		if _, err := w.Write(ctx, bigPayload); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	segsBefore := w.Segments()
	if segsBefore < 2 {
		t.Skipf("not enough segments for truncation test (%d)", segsBefore)
	}

	lastSeq := w.LastSequence()
	// Truncate everything before the last sequence.
	if err := w.Truncate(ctx, lastSeq); err != nil {
		t.Fatalf("Truncate: %v", err)
	}

	segsAfter := w.Segments()
	if segsAfter >= segsBefore {
		t.Errorf("Truncate did not remove segments: before=%d, after=%d", segsBefore, segsAfter)
	}
}

func TestCheckpoint(t *testing.T) {
	dir := t.TempDir()
	w, err := New(Config{
		Path:        dir,
		SegmentSize: 64,
		SyncWrites:  false,
		MaxSegments: 3,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer w.Close()
	ctx := context.Background()

	bigPayload := make([]byte, 40)
	for i := 0; i < 20; i++ {
		if _, err := w.Write(ctx, bigPayload); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	if err := w.Checkpoint(ctx); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}

	// After checkpoint, segments should be <= MaxSegments.
	if segs := w.Segments(); segs > 3 {
		t.Errorf("segments after checkpoint = %d, want <= 3", segs)
	}
}

func TestRecovery(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Path:        dir,
		SegmentSize: 4096,
		SyncWrites:  true,
		MaxSegments: 5,
	}

	// Write entries, close, re-open.
	w1, err := New(cfg)
	if err != nil {
		t.Fatalf("New (first): %v", err)
	}
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if _, err := w1.Write(ctx, []byte{byte(i)}); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	if err := w1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Re-open and verify recovery.
	w2, err := New(cfg)
	if err != nil {
		t.Fatalf("New (second): %v", err)
	}
	defer w2.Close()

	if last := w2.LastSequence(); last != 5 {
		t.Errorf("recovered LastSequence = %d, want 5", last)
	}

	// Should be able to read old entries.
	got, err := w2.Read(ctx, 3)
	if err != nil {
		t.Fatalf("Read after recovery: %v", err)
	}
	if len(got) != 1 || got[0] != 2 {
		t.Errorf("Read seq 3 = %v, want [2]", got)
	}

	// Write new entry after recovery and verify sequence continues.
	seq, err := w2.Write(ctx, []byte{0xFF})
	if err != nil {
		t.Fatalf("Write after recovery: %v", err)
	}
	if seq != 6 {
		t.Errorf("first seq after recovery = %d, want 6", seq)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Path == "" {
		t.Error("DefaultConfig Path is empty")
	}
	if cfg.SegmentSize <= 0 {
		t.Error("DefaultConfig SegmentSize <= 0")
	}
	if cfg.MaxSegments <= 0 {
		t.Error("DefaultConfig MaxSegments <= 0")
	}
	if !cfg.SyncWrites {
		t.Error("DefaultConfig SyncWrites should be true")
	}
}

// ---------- Segment-level tests ----------

func TestSegment_CreateAndWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.wal")
	seg, err := CreateSegment(path, false)
	if err != nil {
		t.Fatalf("CreateSegment: %v", err)
	}
	defer seg.Close()

	if seg.Path() != path {
		t.Errorf("Path = %q, want %q", seg.Path(), path)
	}
	if !seg.IsEmpty() {
		t.Error("new segment should be empty")
	}

	if err := seg.Write(1, []byte("entry-1")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if seg.IsEmpty() {
		t.Error("segment should not be empty after write")
	}
}

func TestSegment_ReadAndReadAll(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.wal")
	seg, err := CreateSegment(path, false)
	if err != nil {
		t.Fatalf("CreateSegment: %v", err)
	}
	defer seg.Close()

	entries := []struct {
		seq  uint64
		data string
	}{
		{1, "alpha"},
		{2, "bravo"},
		{3, "charlie"},
	}
	for _, e := range entries {
		if err := seg.Write(e.seq, []byte(e.data)); err != nil {
			t.Fatalf("Write seq %d: %v", e.seq, err)
		}
	}

	// Read individual entry.
	got, err := seg.Read(2)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != "bravo" {
		t.Errorf("Read seq 2 = %q, want %q", got, "bravo")
	}

	// Read non-existent.
	_, err = seg.Read(99)
	if err != ErrSequenceNotFound {
		t.Errorf("Read non-existent = %v, want ErrSequenceNotFound", err)
	}

	// ReadAll.
	all, err := seg.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("ReadAll = %d entries, want 3", len(all))
	}
	for i, e := range entries {
		if all[i].Sequence != e.seq || string(all[i].Data) != e.data {
			t.Errorf("ReadAll[%d] = {%d, %q}, want {%d, %q}",
				i, all[i].Sequence, all[i].Data, e.seq, e.data)
		}
	}
}

func TestSegment_Scan(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.wal")
	seg, err := CreateSegment(path, false)
	if err != nil {
		t.Fatalf("CreateSegment: %v", err)
	}
	defer seg.Close()

	for i := uint64(1); i <= 5; i++ {
		if err := seg.Write(i, []byte{byte(i)}); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	var scanned []uint64
	err = seg.Scan(3, func(seq uint64, data []byte) error {
		scanned = append(scanned, seq)
		return nil
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(scanned) != 3 {
		t.Fatalf("scanned %d, want 3", len(scanned))
	}
}

func TestSegment_EntryCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.wal")
	seg, err := CreateSegment(path, false)
	if err != nil {
		t.Fatalf("CreateSegment: %v", err)
	}
	defer seg.Close()

	count, err := seg.EntryCount()
	if err != nil {
		t.Fatalf("EntryCount: %v", err)
	}
	if count != 0 {
		t.Errorf("empty segment count = %d, want 0", count)
	}

	for i := uint64(1); i <= 4; i++ {
		seg.Write(i, []byte("data"))
	}

	count, err = seg.EntryCount()
	if err != nil {
		t.Fatalf("EntryCount: %v", err)
	}
	if count != 4 {
		t.Errorf("count = %d, want 4", count)
	}
}

func TestSegment_FirstLastSequence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.wal")
	seg, err := CreateSegment(path, false)
	if err != nil {
		t.Fatalf("CreateSegment: %v", err)
	}
	defer seg.Close()

	for i := uint64(10); i <= 13; i++ {
		seg.Write(i, []byte("d"))
	}

	if first := seg.FirstSequence(); first != 10 {
		t.Errorf("FirstSequence = %d, want 10", first)
	}
	if last := seg.LastSequence(); last != 13 {
		t.Errorf("LastSequence = %d, want 13", last)
	}
}

func TestSegment_IsFull(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.wal")
	seg, err := CreateSegment(path, false)
	if err != nil {
		t.Fatalf("CreateSegment: %v", err)
	}
	defer seg.Close()

	if seg.IsFull(100) {
		t.Error("empty segment should not be full")
	}

	// Write enough to exceed the limit.
	bigData := make([]byte, 60)
	seg.Write(1, bigData)
	seg.Write(2, bigData)

	if !seg.IsFull(100) {
		t.Error("segment should be full after large writes")
	}
}

func TestSegment_OpenExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.wal")

	// Create and write.
	seg1, err := CreateSegment(path, true)
	if err != nil {
		t.Fatalf("CreateSegment: %v", err)
	}
	seg1.Write(1, []byte("one"))
	seg1.Write(2, []byte("two"))
	seg1.Close()

	// Re-open.
	seg2, err := OpenSegment(path, false)
	if err != nil {
		t.Fatalf("OpenSegment: %v", err)
	}
	defer seg2.Close()

	if first := seg2.FirstSequence(); first != 1 {
		t.Errorf("FirstSequence = %d, want 1", first)
	}
	if last := seg2.LastSequence(); last != 2 {
		t.Errorf("LastSequence = %d, want 2", last)
	}

	got, err := seg2.Read(1)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != "one" {
		t.Errorf("Read = %q, want %q", got, "one")
	}
}

func TestSegment_Close_RejectsOperations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.wal")
	seg, err := CreateSegment(path, false)
	if err != nil {
		t.Fatalf("CreateSegment: %v", err)
	}
	seg.Close()

	if err := seg.Write(1, []byte("data")); err != ErrCorruptedEntry {
		t.Errorf("Write after close = %v, want ErrCorruptedEntry", err)
	}
	if _, err := seg.Read(1); err != ErrCorruptedEntry {
		t.Errorf("Read after close = %v, want ErrCorruptedEntry", err)
	}
	if err := seg.Scan(0, nil); err != ErrCorruptedEntry {
		t.Errorf("Scan after close = %v, want ErrCorruptedEntry", err)
	}
	if _, err := seg.ReadAll(); err != ErrCorruptedEntry {
		t.Errorf("ReadAll after close = %v, want ErrCorruptedEntry", err)
	}
	if _, err := seg.EntryCount(); err != ErrCorruptedEntry {
		t.Errorf("EntryCount after close = %v, want ErrCorruptedEntry", err)
	}
	if err := seg.Truncate(0); err != ErrCorruptedEntry {
		t.Errorf("Truncate after close = %v, want ErrCorruptedEntry", err)
	}
	if err := seg.Repair(); err != ErrCorruptedEntry {
		t.Errorf("Repair after close = %v, want ErrCorruptedEntry", err)
	}
}

func TestSegment_Close_Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.wal")
	seg, _ := CreateSegment(path, false)
	if err := seg.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := seg.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestSegment_Sync(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.wal")
	seg, _ := CreateSegment(path, false)
	defer seg.Close()

	seg.Write(1, []byte("data"))
	if err := seg.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}
}

func TestSegment_Truncate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.wal")
	seg, _ := CreateSegment(path, false)
	defer seg.Close()

	seg.Write(1, []byte("first"))
	seg.Write(2, []byte("second"))

	if err := seg.Truncate(0); err != nil {
		t.Fatalf("Truncate: %v", err)
	}
	if !seg.IsEmpty() {
		t.Error("segment should be empty after truncate to 0")
	}
}

func TestSegment_Repair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.wal")
	seg, _ := CreateSegment(path, true)

	seg.Write(1, []byte("valid1"))
	seg.Write(2, []byte("valid2"))

	// Append a corrupt entry: valid header but bad CRC, with data long
	// enough that OpenSegment's scanSequences can read and skip it.
	badData := []byte("corrupt-payload!")
	buf := make([]byte, entryHeaderSize+len(badData))
	binary.BigEndian.PutUint64(buf[0:8], 3)
	binary.BigEndian.PutUint32(buf[8:12], uint32(len(badData)))
	binary.BigEndian.PutUint32(buf[12:16], 0xBADBAD) // wrong CRC
	copy(buf[16:], badData)

	// Flush the segment writer and write raw bytes to underlying file.
	seg.Sync()
	f, _ := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0644)
	f.Write(buf)
	f.Close()
	seg.Close()

	seg2, err := OpenSegment(path, false)
	if err != nil {
		t.Fatalf("OpenSegment after corruption: %v", err)
	}
	defer seg2.Close()

	if err := seg2.Repair(); err != nil {
		t.Fatalf("Repair: %v", err)
	}

	// Valid entries should survive; the corrupt third entry should be removed.
	entries, err := seg2.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll after repair: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("entries after repair = %d, want 2", len(entries))
	}
}

func TestSegment_Repair_CorruptedCRC(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.wal")
	seg, _ := CreateSegment(path, true)
	seg.Write(1, []byte("good"))
	seg.Close()

	// Overwrite the file with a valid header but bad CRC for a second entry.
	f, _ := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0644)
	badData := []byte("bad!")
	buf := make([]byte, entryHeaderSize+len(badData))
	binary.BigEndian.PutUint64(buf[0:8], 2)
	binary.BigEndian.PutUint32(buf[8:12], uint32(len(badData)))
	binary.BigEndian.PutUint32(buf[12:16], 0xDEADBEEF) // wrong CRC
	copy(buf[16:], badData)
	f.Write(buf)
	f.Close()

	seg2, _ := OpenSegment(path, false)
	defer seg2.Close()

	if err := seg2.Repair(); err != nil {
		t.Fatalf("Repair: %v", err)
	}

	entries, err := seg2.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll after repair: %v", err)
	}
	// Only the first valid entry should remain.
	if len(entries) != 1 {
		t.Errorf("entries after CRC repair = %d, want 1", len(entries))
	}
}

// ---------- encodeEntry / internal tests ----------

func TestEncodeEntry(t *testing.T) {
	data := []byte("test-data")
	buf := encodeEntry(42, data)

	if len(buf) != entryHeaderSize+len(data) {
		t.Fatalf("encoded length = %d, want %d", len(buf), entryHeaderSize+len(data))
	}

	seq := binary.BigEndian.Uint64(buf[0:8])
	if seq != 42 {
		t.Errorf("sequence = %d, want 42", seq)
	}

	length := binary.BigEndian.Uint32(buf[8:12])
	if length != uint32(len(data)) {
		t.Errorf("length = %d, want %d", length, len(data))
	}

	crc := binary.BigEndian.Uint32(buf[12:16])
	expectedCRC := crc32.ChecksumIEEE(data)
	if crc != expectedCRC {
		t.Errorf("CRC = %d, want %d", crc, expectedCRC)
	}

	if string(buf[16:]) != string(data) {
		t.Errorf("data = %q, want %q", buf[16:], data)
	}
}

func TestEncodeEntry_EmptyData(t *testing.T) {
	buf := encodeEntry(1, []byte{})
	if len(buf) != entryHeaderSize {
		t.Errorf("encoded empty data length = %d, want %d", len(buf), entryHeaderSize)
	}
}

// ---------- Edge cases ----------

func TestWrite_EmptyData(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()

	seq, err := w.Write(ctx, []byte{})
	if err != nil {
		t.Fatalf("Write empty: %v", err)
	}

	got, err := w.Read(ctx, seq)
	if err != nil {
		t.Fatalf("Read empty: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Read empty = %v, want empty slice", got)
	}
}

func TestWrite_LargeData(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()

	data := make([]byte, 1024*1024) // 1MB
	for i := range data {
		data[i] = byte(i % 256)
	}

	seq, err := w.Write(ctx, data)
	if err != nil {
		t.Fatalf("Write large: %v", err)
	}

	got, err := w.Read(ctx, seq)
	if err != nil {
		t.Fatalf("Read large: %v", err)
	}
	if len(got) != len(data) {
		t.Fatalf("Read large length = %d, want %d", len(got), len(data))
	}
	for i := range data {
		if got[i] != data[i] {
			t.Fatalf("data mismatch at byte %d", i)
		}
	}
}

func TestScan_FromZero(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		w.Write(ctx, []byte{byte(i)})
	}

	var count int
	err := w.Scan(ctx, 0, func(seq uint64, data []byte) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("Scan from 0: %v", err)
	}
	if count != 3 {
		t.Errorf("scanned %d entries from 0, want 3", count)
	}
}

func TestScan_Empty(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()

	var count int
	err := w.Scan(ctx, 0, func(seq uint64, data []byte) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("Scan empty: %v", err)
	}
	if count != 0 {
		t.Errorf("scanned %d entries from empty WAL, want 0", count)
	}
}

func TestOpenSegment_NonExistent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-file.wal")
	_, err := OpenSegment(path, false)
	if err == nil {
		t.Error("OpenSegment non-existent should fail")
	}
}

func TestCreateSegment_InvalidPath(t *testing.T) {
	_, err := CreateSegment("/no/such/directory/test.wal", false)
	if err == nil {
		t.Error("CreateSegment invalid path should fail")
	}
}

func TestNew_InvalidPath(t *testing.T) {
	// Use a path under /dev/null which can't be a directory.
	_, err := New(Config{
		Path:        "/dev/null/impossible",
		SegmentSize: 4096,
		MaxSegments: 5,
	})
	if err == nil {
		t.Error("New with invalid path should fail")
	}
}

func TestSegment_SyncWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sync.wal")
	seg, err := CreateSegment(path, true)
	if err != nil {
		t.Fatalf("CreateSegment: %v", err)
	}
	defer seg.Close()

	// Write with sync enabled should not error.
	if err := seg.Write(1, []byte("synced")); err != nil {
		t.Fatalf("Write with sync: %v", err)
	}

	got, err := seg.Read(1)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != "synced" {
		t.Errorf("Read = %q, want %q", got, "synced")
	}
}
