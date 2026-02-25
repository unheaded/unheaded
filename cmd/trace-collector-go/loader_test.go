package main

import (
	"testing"
)

// ── MockBPFLoader tests ─────────────────────────────────────────────────────

func TestMockBPFLoader_LoadAndAttach(t *testing.T) {
	loader := NewMockBPFLoader()

	// Should not be loaded initially
	if loader.IsLoaded() {
		t.Error("expected not loaded initially")
	}

	// Load
	if err := loader.Load("/path/to/packet_marker.bpf.o"); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !loader.IsLoaded() {
		t.Error("expected loaded after Load()")
	}

	// Attach
	if err := loader.AttachXDP("eth0"); err != nil {
		t.Fatalf("AttachXDP: %v", err)
	}
	if !loader.IsAttached() {
		t.Error("expected attached after AttachXDP()")
	}
	if loader.Interface() != "eth0" {
		t.Errorf("Interface() = %q, want \"eth0\"", loader.Interface())
	}

	// Close
	if err := loader.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if loader.IsAttached() {
		t.Error("expected detached after Close()")
	}
}

func TestMockBPFLoader_AttachWithoutLoad(t *testing.T) {
	loader := NewMockBPFLoader()

	err := loader.AttachXDP("eth0")
	if err == nil {
		t.Error("expected error when attaching without loading")
	}
}

func TestMockBPFLoader_LoadAfterClose(t *testing.T) {
	loader := NewMockBPFLoader()
	loader.Close()

	err := loader.Load("/path/to/prog.o")
	if err == nil {
		t.Error("expected error when loading after close")
	}
}

func TestMockBPFLoader_MapsNotNil(t *testing.T) {
	loader := NewMockBPFLoader()

	if loader.GetTraceMap() == nil {
		t.Error("GetTraceMap() returned nil")
	}
	if loader.GetStatsMap() == nil {
		t.Error("GetStatsMap() returned nil")
	}
	if loader.GetFlowStateMap() == nil {
		t.Error("GetFlowStateMap() returned nil")
	}
}

// ── MockMapReader tests ─────────────────────────────────────────────────────

func TestMockMapReader_InsertAndLookup(t *testing.T) {
	m := NewMockMapReader()

	key := []byte("test-key")
	value := []byte("test-value")

	m.Insert(key, value)

	got, err := m.Lookup(key)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if string(got) != string(value) {
		t.Errorf("Lookup = %q, want %q", got, value)
	}
}

func TestMockMapReader_LookupMissing(t *testing.T) {
	m := NewMockMapReader()

	got, err := m.Lookup([]byte("missing"))
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got != nil {
		t.Errorf("Lookup = %v, want nil", got)
	}
}

func TestMockMapReader_Delete(t *testing.T) {
	m := NewMockMapReader()

	key := []byte("key")
	m.Insert(key, []byte("value"))

	if m.Len() != 1 {
		t.Errorf("Len() = %d, want 1", m.Len())
	}

	if err := m.Delete(key); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if m.Len() != 0 {
		t.Errorf("Len() = %d after delete, want 0", m.Len())
	}
}

func TestMockMapReader_Iterate(t *testing.T) {
	m := NewMockMapReader()

	m.Insert([]byte("k1"), []byte("v1"))
	m.Insert([]byte("k2"), []byte("v2"))
	m.Insert([]byte("k3"), []byte("v3"))

	count := 0
	err := m.Iterate(func(key, value []byte) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("Iterate: %v", err)
	}
	if count != 3 {
		t.Errorf("Iterate visited %d entries, want 3", count)
	}
}

func TestMockMapReader_Len(t *testing.T) {
	m := NewMockMapReader()

	if m.Len() != 0 {
		t.Errorf("initial Len() = %d, want 0", m.Len())
	}

	m.Insert([]byte("a"), []byte("1"))
	m.Insert([]byte("b"), []byte("2"))

	if m.Len() != 2 {
		t.Errorf("Len() = %d, want 2", m.Len())
	}
}

func TestMockMapReader_InsertOverwrite(t *testing.T) {
	m := NewMockMapReader()

	key := []byte("key")
	m.Insert(key, []byte("first"))
	m.Insert(key, []byte("second"))

	got, err := m.Lookup(key)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if string(got) != "second" {
		t.Errorf("Lookup after overwrite = %q, want \"second\"", got)
	}
	if m.Len() != 1 {
		t.Errorf("Len() = %d, want 1 (overwrite should not duplicate)", m.Len())
	}
}

func TestMockMapReader_LookupReturnsCopy(t *testing.T) {
	m := NewMockMapReader()

	key := []byte("key")
	m.Insert(key, []byte("original"))

	got, _ := m.Lookup(key)
	// Mutate the returned value
	got[0] = 'X'

	// Verify original is unchanged
	got2, _ := m.Lookup(key)
	if string(got2) != "original" {
		t.Error("Lookup should return a copy; original was mutated")
	}
}
