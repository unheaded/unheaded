// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package compute

import (
	"context"
	"encoding/binary"
	"testing"
	"time"
)

func TestNewMemoryService_Defaults(t *testing.T) {
	bpf := NewMemoryBPFMap(0)
	ms := NewMemoryService(DefaultConfig(), bpf)

	if ms.config.RingSize != DefaultRingSize {
		t.Errorf("RingSize = %d, want %d", ms.config.RingSize, DefaultRingSize)
	}
	if ms.config.PageSize != DefaultPageSize {
		t.Errorf("PageSize = %d, want %d", ms.config.PageSize, DefaultPageSize)
	}
	if ms.config.PrefetchN != DefaultPrefetchN {
		t.Errorf("PrefetchN = %d, want %d", ms.config.PrefetchN, DefaultPrefetchN)
	}
	if ms.config.L1MaxPages != DefaultL1MaxPages {
		t.Errorf("L1MaxPages = %d, want %d", ms.config.L1MaxPages, DefaultL1MaxPages)
	}
}

func TestNewMemoryService_ZeroPrefetch(t *testing.T) {
	bpf := NewMemoryBPFMap(0)
	ms := NewMemoryService(Config{
		RingSize:   1024,
		PageSize:   256,
		PrefetchN:  0, // explicitly no prefetch
		L1MaxPages: 4,
	}, bpf)

	if ms.config.PrefetchN != 0 {
		t.Errorf("PrefetchN = %d, want 0 (explicitly set)", ms.config.PrefetchN)
	}
}

func TestLoadData_And_ReadData(t *testing.T) {
	bpf := NewMemoryBPFMap(0)
	ms := NewMemoryService(Config{
		RingSize: 1024,
		PageSize: 256,
	}, bpf)

	data := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE, 0xBA, 0xBE}
	if err := ms.LoadData(100, data); err != nil {
		t.Fatalf("LoadData: %v", err)
	}

	got := ms.ReadData(100, 8)
	for i, b := range got {
		if b != data[i] {
			t.Errorf("ReadData[%d] = 0x%02X, want 0x%02X", i, b, data[i])
		}
	}
}

func TestLoadData_OutOfBounds(t *testing.T) {
	bpf := NewMemoryBPFMap(0)
	ms := NewMemoryService(Config{
		RingSize: 256,
		PageSize: 64,
	}, bpf)

	data := make([]byte, 300)
	err := ms.LoadData(0, data)
	if err == nil {
		t.Fatal("expected error for out-of-bounds load")
	}
}

func TestReadWord(t *testing.T) {
	bpf := NewMemoryBPFMap(0)
	ms := NewMemoryService(Config{
		RingSize: 1024,
		PageSize: 256,
	}, bpf)

	// Write a known word at byte offset 16.
	word := uint32(0x12345678)
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, word)
	if err := ms.LoadData(16, buf); err != nil {
		t.Fatalf("LoadData: %v", err)
	}

	got := ms.ReadWord(16)
	if got != word {
		t.Errorf("ReadWord(16) = 0x%08X, want 0x%08X", got, word)
	}
}

func TestHandleCacheMiss_LoadsPage(t *testing.T) {
	bpf := NewMemoryBPFMap(0)
	cfg := Config{
		RingSize:   4096,
		PageSize:   256, // 64 words per page
		PrefetchN:  0,   // no prefetch for this test
		L1MaxPages: 16,
	}
	ms := NewMemoryService(cfg, bpf)

	// Load some data into L2 at page 0 (bytes 0-255).
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}
	if err := ms.LoadData(0, data); err != nil {
		t.Fatalf("LoadData: %v", err)
	}

	// Trigger a cache miss for address 42 (page 0).
	event := CacheMissEvent{
		FlowLabel: 1,
		HopID:     0,
		Address:   42,
		Timestamp: time.Now().UnixNano(),
	}
	if err := ms.HandleCacheMiss(event); err != nil {
		t.Fatalf("HandleCacheMiss: %v", err)
	}

	// Verify page was loaded into BPF map.
	// Word at byte offset 0 = data[0..3] = {0,1,2,3} = 0x03020100 LE.
	word0, _ := bpf.ReadWord(0)
	expected := binary.LittleEndian.Uint32([]byte{0, 1, 2, 3})
	if word0 != expected {
		t.Errorf("BPF word[0] = 0x%08X, want 0x%08X", word0, expected)
	}

	// Word at byte offset 40 = word address 10 = data[40..43].
	word10, _ := bpf.ReadWord(10)
	expected10 := binary.LittleEndian.Uint32([]byte{40, 41, 42, 43})
	if word10 != expected10 {
		t.Errorf("BPF word[10] = 0x%08X, want 0x%08X", word10, expected10)
	}

	stats := ms.GetStats()
	if stats.MissCount != 1 {
		t.Errorf("MissCount = %d, want 1", stats.MissCount)
	}
	if stats.L1Pages != 1 {
		t.Errorf("L1Pages = %d, want 1", stats.L1Pages)
	}
}

func TestHandleCacheMiss_Prefetch(t *testing.T) {
	bpf := NewMemoryBPFMap(0)
	cfg := Config{
		RingSize:   4096,
		PageSize:   256,
		PrefetchN:  2,
		L1MaxPages: 16,
	}
	ms := NewMemoryService(cfg, bpf)

	// Fill L2 with recognizable data.
	data := make([]byte, 768) // 3 pages worth
	for i := range data {
		data[i] = byte(i % 256)
	}
	if err := ms.LoadData(0, data); err != nil {
		t.Fatalf("LoadData: %v", err)
	}

	// Miss at address 10 (page 0) should also prefetch pages 1 and 2.
	event := CacheMissEvent{
		FlowLabel: 1,
		HopID:     0,
		Address:   10,
	}
	if err := ms.HandleCacheMiss(event); err != nil {
		t.Fatalf("HandleCacheMiss: %v", err)
	}

	stats := ms.GetStats()
	if stats.L1Pages != 3 {
		t.Errorf("L1Pages = %d, want 3 (1 requested + 2 prefetch)", stats.L1Pages)
	}

	// Verify page 2 was prefetched: first word of page 2 = byte offset 512.
	// Word at word address 512/4 = 128.
	word128, _ := bpf.ReadWord(128)
	expected := binary.LittleEndian.Uint32([]byte{0, 1, 2, 3}) // data[512..515] = {0,1,2,3}
	if word128 != expected {
		t.Errorf("BPF word[128] (page 2) = 0x%08X, want 0x%08X", word128, expected)
	}
}

func TestHandleCacheWrite_UpdatesL2(t *testing.T) {
	bpf := NewMemoryBPFMap(0)
	cfg := Config{
		RingSize:   4096,
		PageSize:   256,
		PrefetchN:  0,
		L1MaxPages: 16,
	}
	ms := NewMemoryService(cfg, bpf)

	// Write a word at address 100.
	event := CacheWriteEvent{
		FlowLabel: 1,
		HopID:     0,
		Address:   100,
		Value:     0xDEADBEEF,
		Width:     4,
	}
	if err := ms.HandleCacheWrite(event); err != nil {
		t.Fatalf("HandleCacheWrite: %v", err)
	}

	// Verify L2 was updated.
	got := ms.ReadWord(100)
	if got != 0xDEADBEEF {
		t.Errorf("L2 word at 100 = 0x%08X, want 0xDEADBEEF", got)
	}

	stats := ms.GetStats()
	if stats.WriteCount != 1 {
		t.Errorf("WriteCount = %d, want 1", stats.WriteCount)
	}
}

func TestHandleCacheWrite_ByteWidth(t *testing.T) {
	bpf := NewMemoryBPFMap(0)
	ms := NewMemoryService(Config{
		RingSize: 1024,
		PageSize: 256,
	}, bpf)

	event := CacheWriteEvent{
		Address: 50,
		Value:   0x42,
		Width:   1,
	}
	if err := ms.HandleCacheWrite(event); err != nil {
		t.Fatalf("HandleCacheWrite: %v", err)
	}

	data := ms.ReadData(50, 1)
	if len(data) != 1 || data[0] != 0x42 {
		t.Errorf("L2 byte at 50 = %v, want [0x42]", data)
	}
}

func TestHandleCacheWrite_HalfwordWidth(t *testing.T) {
	bpf := NewMemoryBPFMap(0)
	ms := NewMemoryService(Config{
		RingSize: 1024,
		PageSize: 256,
	}, bpf)

	event := CacheWriteEvent{
		Address: 60,
		Value:   0xBEEF,
		Width:   2,
	}
	if err := ms.HandleCacheWrite(event); err != nil {
		t.Fatalf("HandleCacheWrite: %v", err)
	}

	data := ms.ReadData(60, 2)
	if len(data) < 2 {
		t.Fatalf("ReadData returned %d bytes, want >= 2", len(data))
	}
	got := binary.LittleEndian.Uint16(data)
	if got != 0xBEEF {
		t.Errorf("L2 halfword at 60 = 0x%04X, want 0xBEEF", got)
	}
}

func TestLRU_Eviction(t *testing.T) {
	bpf := NewMemoryBPFMap(0)
	cfg := Config{
		RingSize:   8192,
		PageSize:   256,
		PrefetchN:  0,
		L1MaxPages: 2, // only 2 pages allowed in L1
	}
	ms := NewMemoryService(cfg, bpf)

	// Fill 3 pages in L2.
	for page := 0; page < 3; page++ {
		data := make([]byte, 256)
		for i := range data {
			data[i] = byte(page + 1)
		}
		if err := ms.LoadData(uint32(page*256), data); err != nil {
			t.Fatalf("LoadData page %d: %v", page, err)
		}
	}

	// Load page 0 into L1.
	ms.HandleCacheMiss(CacheMissEvent{Address: 0, HopID: 0})
	// Load page 1 into L1.
	ms.HandleCacheMiss(CacheMissEvent{Address: 256, HopID: 0})

	stats := ms.GetStats()
	if stats.L1Pages != 2 {
		t.Errorf("after 2 loads: L1Pages = %d, want 2", stats.L1Pages)
	}

	// Load page 2 — should evict page 0 (LRU).
	ms.HandleCacheMiss(CacheMissEvent{Address: 512, HopID: 0})

	stats = ms.GetStats()
	if stats.L1Pages != 2 {
		t.Errorf("after 3rd load: L1Pages = %d, want 2", stats.L1Pages)
	}
	if stats.EvictCount != 1 {
		t.Errorf("EvictCount = %d, want 1", stats.EvictCount)
	}

	// Verify page 2 data is in BPF (word addr 128 = byte 512 / 4).
	word128, _ := bpf.ReadWord(128)
	expected := binary.LittleEndian.Uint32([]byte{3, 3, 3, 3})
	if word128 != expected {
		t.Errorf("page 2 word = 0x%08X, want 0x%08X", word128, expected)
	}
}

func TestLRU_TouchPromotes(t *testing.T) {
	bpf := NewMemoryBPFMap(0)
	cfg := Config{
		RingSize:   8192,
		PageSize:   256,
		PrefetchN:  0,
		L1MaxPages: 2,
	}
	ms := NewMemoryService(cfg, bpf)

	// Fill 3 pages.
	for page := 0; page < 3; page++ {
		data := make([]byte, 256)
		data[0] = byte(page + 1)
		ms.LoadData(uint32(page*256), data)
	}

	// Load page 0, then page 1.
	ms.HandleCacheMiss(CacheMissEvent{Address: 0, HopID: 0})
	ms.HandleCacheMiss(CacheMissEvent{Address: 256, HopID: 0})

	// Touch page 0 again (re-miss promotes in LRU).
	ms.HandleCacheMiss(CacheMissEvent{Address: 0, HopID: 0})

	// Now load page 2 — should evict page 1 (LRU), not page 0.
	ms.HandleCacheMiss(CacheMissEvent{Address: 512, HopID: 0})

	stats := ms.GetStats()
	if stats.EvictCount != 1 {
		t.Errorf("EvictCount = %d, want 1", stats.EvictCount)
	}

	// Page 0 should still be in L1: word 0.
	word0, _ := bpf.ReadWord(0)
	expected0 := binary.LittleEndian.Uint32([]byte{1, 0, 0, 0})
	if word0 != expected0 {
		t.Errorf("page 0 word = 0x%08X, want 0x%08X (should still be cached)", word0, expected0)
	}
}

func TestDirtyWriteback(t *testing.T) {
	bpf := NewMemoryBPFMap(0)
	cfg := Config{
		RingSize:   4096,
		PageSize:   256,
		PrefetchN:  0,
		L1MaxPages: 1, // force eviction on second page load
	}
	ms := NewMemoryService(cfg, bpf)

	// Load page 0 into L2 and L1.
	data := make([]byte, 256)
	ms.LoadData(0, data)
	ms.HandleCacheMiss(CacheMissEvent{Address: 0, HopID: 0})

	// Simulate BPF writing to the cached page (direct to bpf map).
	bpf.WriteWord(0, 0xCAFEBABE)

	// Simulate a write event to mark the page dirty.
	ms.HandleCacheWrite(CacheWriteEvent{Address: 0, Value: 0xCAFEBABE, Width: 4})

	// Load page 1 — forces eviction of page 0. Should writeback dirty data.
	data1 := make([]byte, 256)
	ms.LoadData(256, data1)
	ms.HandleCacheMiss(CacheMissEvent{Address: 256, HopID: 0})

	// Verify page 0's dirty data was written back to L2.
	got := ms.ReadWord(0)
	if got != 0xCAFEBABE {
		t.Errorf("L2 word after writeback = 0x%08X, want 0xCAFEBABE", got)
	}
}

func TestStageAllToL1(t *testing.T) {
	bpf := NewMemoryBPFMap(0)
	cfg := Config{
		RingSize:   2048,
		PageSize:   256,
		PrefetchN:  0,
		L1MaxPages: 10,
	}
	ms := NewMemoryService(cfg, bpf)

	// Write data to pages 0, 2, and 5 (skip 1, 3, 4).
	for _, page := range []int{0, 2, 5} {
		data := make([]byte, 256)
		data[0] = byte(page + 1)
		ms.LoadData(uint32(page*256), data)
	}

	staged, err := ms.StageAllToL1(0)
	if err != nil {
		t.Fatalf("StageAllToL1: %v", err)
	}
	if staged != 3 {
		t.Errorf("staged = %d, want 3", staged)
	}

	stats := ms.GetStats()
	if stats.L1Pages != 3 {
		t.Errorf("L1Pages = %d, want 3", stats.L1Pages)
	}
}

func TestAlignToPage(t *testing.T) {
	bpf := NewMemoryBPFMap(0)
	ms := NewMemoryService(Config{PageSize: 4096}, bpf)

	tests := []struct {
		addr     uint32
		expected uint32
	}{
		{0, 0},
		{1, 0},
		{4095, 0},
		{4096, 4096},
		{4097, 4096},
		{8191, 4096},
		{8192, 8192},
	}

	for _, tt := range tests {
		got := ms.alignToPage(tt.addr)
		if got != tt.expected {
			t.Errorf("alignToPage(%d) = %d, want %d", tt.addr, got, tt.expected)
		}
	}
}

func TestParseEvents(t *testing.T) {
	missJSON := `{"flow_label":42,"hop_id":3,"address":1024,"timestamp":123456789}`
	miss, err := ParseCacheMissEvent(missJSON)
	if err != nil {
		t.Fatalf("ParseCacheMissEvent: %v", err)
	}
	if miss.FlowLabel != 42 || miss.HopID != 3 || miss.Address != 1024 {
		t.Errorf("miss = %+v, unexpected values", miss)
	}

	writeJSON := `{"flow_label":42,"hop_id":3,"address":2048,"value":255,"width":1,"timestamp":123456789}`
	write, err := ParseCacheWriteEvent(writeJSON)
	if err != nil {
		t.Fatalf("ParseCacheWriteEvent: %v", err)
	}
	if write.FlowLabel != 42 || write.Address != 2048 || write.Value != 255 || write.Width != 1 {
		t.Errorf("write = %+v, unexpected values", write)
	}
}

func TestRunEventLoop(t *testing.T) {
	bpf := NewMemoryBPFMap(0)
	cfg := Config{
		RingSize:   4096,
		PageSize:   256,
		PrefetchN:  0,
		L1MaxPages: 16,
	}
	ms := NewMemoryService(cfg, bpf)

	// Pre-load data.
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}
	ms.LoadData(0, data)

	ctx, cancel := context.WithCancel(context.Background())
	missCh := make(chan CacheMissEvent, 10)
	writeCh := make(chan CacheWriteEvent, 10)

	// Start event loop.
	done := make(chan struct{})
	go func() {
		ms.Run(ctx, missCh, writeCh)
		close(done)
	}()

	// Send a miss event.
	missCh <- CacheMissEvent{FlowLabel: 1, HopID: 0, Address: 42}
	time.Sleep(10 * time.Millisecond) // let it process

	// Send a write event.
	writeCh <- CacheWriteEvent{FlowLabel: 1, HopID: 0, Address: 100, Value: 0xFF, Width: 1}
	time.Sleep(10 * time.Millisecond)

	// Stop.
	cancel()
	<-done

	stats := ms.GetStats()
	if stats.MissCount != 1 {
		t.Errorf("MissCount = %d, want 1", stats.MissCount)
	}
	if stats.WriteCount != 1 {
		t.Errorf("WriteCount = %d, want 1", stats.WriteCount)
	}
}

func TestMemoryBPFMap(t *testing.T) {
	m := NewMemoryBPFMap(3)

	// Write 3 entries (at capacity).
	if err := m.WriteWord(0, 100); err != nil {
		t.Fatalf("WriteWord(0): %v", err)
	}
	if err := m.WriteWord(1, 200); err != nil {
		t.Fatalf("WriteWord(1): %v", err)
	}
	if err := m.WriteWord(2, 300); err != nil {
		t.Fatalf("WriteWord(2): %v", err)
	}

	// 4th entry should fail.
	if err := m.WriteWord(3, 400); err == nil {
		t.Error("expected error when writing to full map")
	}

	// Updating existing entry should succeed.
	if err := m.WriteWord(1, 999); err != nil {
		t.Fatalf("WriteWord(1) update: %v", err)
	}

	v, _ := m.ReadWord(1)
	if v != 999 {
		t.Errorf("ReadWord(1) = %d, want 999", v)
	}

	// Missing key returns 0.
	v, _ = m.ReadWord(99)
	if v != 0 {
		t.Errorf("ReadWord(99) = %d, want 0", v)
	}

	// Delete and write new.
	m.DeleteWord(0)
	if err := m.WriteWord(3, 400); err != nil {
		t.Fatalf("WriteWord(3) after delete: %v", err)
	}

	if m.Len() != 3 {
		t.Errorf("Len() = %d, want 3", m.Len())
	}
}

func TestMemoryBPFMap_WritePage(t *testing.T) {
	m := NewMemoryBPFMap(0)

	words := []uint32{0x11, 0x22, 0x33, 0x44}
	if err := m.WritePage(10, words); err != nil {
		t.Fatalf("WritePage: %v", err)
	}

	for i, want := range words {
		got, _ := m.ReadWord(10 + uint32(i))
		if got != want {
			t.Errorf("ReadWord(%d) = 0x%X, want 0x%X", 10+i, got, want)
		}
	}
}

// ── D-006 Cache Miss Handler tests ────────────────────────────────────────────

func TestHandleCacheMissD006_StagesLine(t *testing.T) {
	bpf := NewMemoryBPFMap(0)
	cfg := Config{
		RingSize:      2048,
		PageSize:      256,
		CacheLineSize: 64,
		PrefetchN:     0,
		L1MaxPages:    16,
	}
	ms := NewMemoryService(cfg, bpf)

	// Load test data into L2.
	data := make([]byte, 128)
	for i := range data {
		data[i] = byte(i % 256)
	}
	if err := ms.LoadData(0, data); err != nil {
		t.Fatalf("LoadData: %v", err)
	}

	// Trigger a cache miss for lineAddr 0x40 (64-byte aligned).
	if err := ms.HandleCacheMissD006(1, 0x40); err != nil {
		t.Fatalf("HandleCacheMissD006: %v", err)
	}

	// Verify the line was staged into BPF map.
	// Word address = 0x40 >> 2 = 0x10 (16).
	word16, _ := bpf.ReadWord(16)
	expected := binary.LittleEndian.Uint32([]byte{0x40, 0x41, 0x42, 0x43})
	if word16 != expected {
		t.Errorf("BPF word[16] = 0x%08X, want 0x%08X", word16, expected)
	}

	// Check another word in the line (e.g., word 18 = byte offset 0x48).
	word18, _ := bpf.ReadWord(18)
	expected18 := binary.LittleEndian.Uint32([]byte{0x48, 0x49, 0x4A, 0x4B})
	if word18 != expected18 {
		t.Errorf("BPF word[18] = 0x%08X, want 0x%08X", word18, expected18)
	}
}

func TestHandleCacheMissD006_Prefetch(t *testing.T) {
	bpf := NewMemoryBPFMap(0)
	cfg := Config{
		RingSize:      512,
		PageSize:      256,
		CacheLineSize: 64,
		PrefetchN:     3, // prefetch 3 additional lines
		L1MaxPages:    16,
	}
	ms := NewMemoryService(cfg, bpf)

	// Load test data (2 cache lines = 128 bytes).
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}
	if err := ms.LoadData(0, data); err != nil {
		t.Fatalf("LoadData: %v", err)
	}

	// Miss at line 0 should prefetch lines 1, 2, 3.
	if err := ms.HandleCacheMissD006(1, 0); err != nil {
		t.Fatalf("HandleCacheMissD006: %v", err)
	}

	// Verify line 2 was prefetched (byte offset 128 = 0x80).
	// Word address = 0x80 >> 2 = 32.
	word32, _ := bpf.ReadWord(32)
	expected := binary.LittleEndian.Uint32([]byte{0x80, 0x81, 0x82, 0x83})
	if word32 != expected {
		t.Errorf("BPF word[32] (line 2) = 0x%08X, want 0x%08X", word32, expected)
	}
}

func TestHandleCacheMissD006_AlignmentCorrection(t *testing.T) {
	bpf := NewMemoryBPFMap(0)
	cfg := Config{
		RingSize:      2048,
		PageSize:      256,
		CacheLineSize: 64,
		PrefetchN:     0,
		L1MaxPages:    16,
	}
	ms := NewMemoryService(cfg, bpf)

	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}
	ms.LoadData(0, data)

	// Pass misaligned address; handler should align to 64-byte boundary.
	if err := ms.HandleCacheMissD006(1, 0x52); err != nil {
		t.Fatalf("HandleCacheMissD006: %v", err)
	}

	// Should be aligned to 0x40 (0x52 & ^0x3F = 0x40).
	word16, _ := bpf.ReadWord(16)
	expected := binary.LittleEndian.Uint32([]byte{0x40, 0x41, 0x42, 0x43})
	if word16 != expected {
		t.Errorf("misaligned address should be corrected to 0x40")
	}
}

// ── D-007 Dirty Writeback Handler tests ──────────────────────────────────────

func TestDirtyBitmap_MarkDirty(t *testing.T) {
	bpf := NewMemoryBPFMap(0)
	cfg := DefaultConfig()
	cfg.CacheLineSize = 64
	ms := NewMemoryService(cfg, bpf)

	// Load initial data.
	data := make([]byte, 128)
	for i := range data {
		data[i] = byte(i)
	}
	ms.LoadData(0, data)

	db := NewDirtyBitmap(ms, cfg)

	// Mark a 4-byte write at offset 16 within line 0.
	if err := db.MarkDirty(1, 0, 16, 4, 0xDEADBEEF); err != nil {
		t.Fatalf("MarkDirty: %v", err)
	}

	// Verify the dirty line contains the patched bytes.
	dirty := db.FlushAll()
	if len(dirty) != 1 || len(dirty[1]) != 1 {
		t.Fatalf("expected 1 dirty flow with 1 line, got %+v", dirty)
	}

	line := dirty[1][0]
	got := binary.LittleEndian.Uint32(line[16:20])
	if got != 0xDEADBEEF {
		t.Errorf("dirty line[16:20] = 0x%08X, want 0xDEADBEEF", got)
	}

	// Verify unmodified bytes are still there.
	got0 := binary.LittleEndian.Uint32(line[0:4])
	expected0 := binary.LittleEndian.Uint32([]byte{0x00, 0x01, 0x02, 0x03})
	if got0 != expected0 {
		t.Errorf("dirty line[0:4] = 0x%08X, want 0x%08X", got0, expected0)
	}
}

func TestDirtyBitmap_RaceCondition(t *testing.T) {
	bpf := NewMemoryBPFMap(0)
	cfg := DefaultConfig()
	cfg.CacheLineSize = 64
	ms := NewMemoryService(cfg, bpf)

	data := make([]byte, 256)
	ms.LoadData(0, data)

	db := NewDirtyBitmap(ms, cfg)

	// Launch 10 goroutines writing to the same dirty bitmap.
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				err := db.MarkDirty(
					uint32(id),
					uint32(j*64),
					(j*4) % 60, // offset within 64-byte line
					4,
					uint32(0xDEAD0000|uint32(id*100+j)),
				)
				done <- err
			}
		}(i)
	}

	// Collect all errors.
	for i := 0; i < 100; i++ {
		if err := <-done; err != nil {
			t.Fatalf("MarkDirty error: %v", err)
		}
	}

	// Verify dirty bitmap is consistent (no panics, no data races with -race).
	dirty := db.FlushAll()
	if len(dirty) != 10 {
		t.Errorf("expected 10 flows, got %d", len(dirty))
	}
}

func TestDirtyWritebackHandler_Flush(t *testing.T) {
	bpf := NewMemoryBPFMap(0)
	cfg := Config{
		RingSize:        2048,
		PageSize:        256,
		CacheLineSize:   64,
		WritebackPeriod: 100 * time.Millisecond,
		L1MaxPages:      16,
	}
	ms := NewMemoryService(cfg, bpf)

	// Initialize L2 memory.
	data := make([]byte, 256)
	ms.LoadData(0, data)

	dwh := NewDirtyWritebackHandler(ms, cfg, nil)

	// Accumulate 3 writes to the same cache line.
	if err := dwh.HandleCacheWriteD007(1, 0x10, 4, 0xDEADBEEF); err != nil {
		t.Fatalf("HandleCacheWriteD007: %v", err)
	}
	if err := dwh.HandleCacheWriteD007(1, 0x20, 4, 0xCAFEBABE); err != nil {
		t.Fatalf("HandleCacheWriteD007: %v", err)
	}
	if err := dwh.HandleCacheWriteD007(1, 0x30, 4, 0x12345678); err != nil {
		t.Fatalf("HandleCacheWriteD007: %v", err)
	}

	// Flush dirty pages.
	event, err := dwh.FlushDirtyPages()
	if err != nil {
		t.Fatalf("FlushDirtyPages: %v", err)
	}

	if event.PageCount != 1 {
		t.Errorf("PageCount = %d, want 1", event.PageCount)
	}

	// Verify L2 was updated with dirty data.
	got := ms.ReadWord(0x10)
	if got != 0xDEADBEEF {
		t.Errorf("L2 word at 0x10 = 0x%08X, want 0xDEADBEEF", got)
	}

	got = ms.ReadWord(0x20)
	if got != 0xCAFEBABE {
		t.Errorf("L2 word at 0x20 = 0x%08X, want 0xCAFEBABE", got)
	}

	got = ms.ReadWord(0x30)
	if got != 0x12345678 {
		t.Errorf("L2 word at 0x30 = 0x%08X, want 0x12345678", got)
	}
}

func TestDirtyWritebackHandler_MultiFlow(t *testing.T) {
	bpf := NewMemoryBPFMap(0)
	cfg := Config{
		RingSize:        2048,
		PageSize:        256,
		CacheLineSize:   64,
		WritebackPeriod: 100 * time.Millisecond,
		L1MaxPages:      16,
	}
	ms := NewMemoryService(cfg, bpf)

	data := make([]byte, 512)
	ms.LoadData(0, data)

	dwh := NewDirtyWritebackHandler(ms, cfg, nil)

	// Write from flow 1, line 0.
	dwh.HandleCacheWriteD007(1, 0x10, 4, 0xAAAAAAAA)

	// Write from flow 2, line 64.
	dwh.HandleCacheWriteD007(2, 0x40+0x20, 4, 0xBBBBBBBB)

	// Write from flow 3, line 128.
	dwh.HandleCacheWriteD007(3, 0x80+0x30, 4, 0xCCCCCCCC)

	event, _ := dwh.FlushDirtyPages()

	// Should have 3 dirty lines across 3 flows.
	if event.PageCount != 3 {
		t.Errorf("PageCount = %d, want 3", event.PageCount)
	}

	// Verify each flow's write was persisted.
	got1 := ms.ReadWord(0x10)
	if got1 != 0xAAAAAAAA {
		t.Errorf("flow 1, L2 = 0x%08X, want 0xAAAAAAAA", got1)
	}

	got2 := ms.ReadWord(0x40 + 0x20)
	if got2 != 0xBBBBBBBB {
		t.Errorf("flow 2, L2 = 0x%08X, want 0xBBBBBBBB", got2)
	}

	got3 := ms.ReadWord(0x80 + 0x30)
	if got3 != 0xCCCCCCCC {
		t.Errorf("flow 3, L2 = 0x%08X, want 0xCCCCCCCC", got3)
	}
}
