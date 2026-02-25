// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

//go:build linux
// +build linux

package maploader

import (
	"testing"
)

// ExampleNewMapLoader demonstrates creating a MapLoader instance
func TestExampleNewMapLoader(t *testing.T) {
	// In production, mapFD comes from ebpf.NativeLoader.GetMap()
	// For this example, we'll use -1 (which would fail in real syscalls)
	mapFD := -1
	mapName := "unhd_hmac_keys"

	loader := NewMapLoader(mapFD, mapName)
	if loader == nil {
		t.Fatalf("NewMapLoader returned nil")
	}

	if loader.mapFD != mapFD {
		t.Errorf("mapFD mismatch: expected %d, got %d", mapFD, loader.mapFD)
	}

	if loader.name != mapName {
		t.Errorf("name mismatch: expected %s, got %s", mapName, loader.name)
	}
}

// ExampleLoadMap demonstrates loading a single key-value pair
func TestExampleLoadMap(t *testing.T) {
	// Minimal example - in production, mapFD would be a valid FD
	loader := NewMapLoader(-1, "test_map")

	// Simplified structures for example
	type SimpleKey struct {
		ID uint32
	}

	type SimpleValue struct {
		Count uint32
	}

	key := SimpleKey{ID: 1}
	value := SimpleValue{Count: 100}

	// Note: This will fail with "maploader: test_map: map update: bad file descriptor"
	// because we're using -1 as the FD. In real usage, mapFD would be from GetMap()
	err := loader.LoadMap(key, value)
	if err == nil {
		t.Error("expected error with invalid FD")
	}
}

// ExampleLoadMapBatch demonstrates loading multiple entries
func TestExampleLoadMapBatch(t *testing.T) {
	loader := NewMapLoader(-1, "test_map")

	type SimpleKey struct {
		ID uint32
	}

	type SimpleValue struct {
		Count uint32
	}

	entries := [][2]interface{}{
		{SimpleKey{ID: 1}, SimpleValue{Count: 100}},
		{SimpleKey{ID: 2}, SimpleValue{Count: 200}},
		{SimpleKey{ID: 3}, SimpleValue{Count: 300}},
	}

	err := loader.LoadMapBatch(entries)
	if err == nil {
		t.Error("expected error with invalid FD")
	}
}

// ExampleLoadMapArray demonstrates loading entries into an ARRAY-type map
func TestExampleLoadMapArray(t *testing.T) {
	loader := NewMapLoader(-1, "unhd_flow_types")

	type FlowTypeEntry struct {
		Type  uint8
		Flags uint8
		_     [2]byte
	}

	entries := []interface{}{
		FlowTypeEntry{Type: 0x00, Flags: 0x00}, // Index 0: Control flow
		FlowTypeEntry{Type: 0x01, Flags: 0x01}, // Index 1: Data flow
		FlowTypeEntry{Type: 0x02, Flags: 0x00}, // Index 2: Prefetch flow
	}

	err := loader.LoadMapArray(entries)
	if err == nil {
		t.Error("expected error with invalid FD")
	}
}

// ExampleLoadErrorCounters demonstrates initializing error counters
func TestExampleLoadErrorCounters(t *testing.T) {
	loader := NewMapLoader(-1, "unhd_error_counters")

	// LoadErrorCounters initializes error codes 0-255 with zero counters
	// This is a helper that doesn't require external data
	err := loader.LoadErrorCounters()
	if err == nil {
		t.Error("expected error with invalid FD")
	}
}

// ExampleDocumentation shows the typical workflow for populating maps
func TestExampleDocumentation(t *testing.T) {
	// Step 1: Get the BPF loader
	// loader, err := ebpf.NewNativeLoader(config)
	// if err != nil { /* handle error */ }

	// Step 2: Load eBPF programs
	// _, err := loader.Load(ctx, "/path/to/program.bpf.o")
	// if err != nil { /* handle error */ }

	// Step 3: Get map file descriptors (example)
	// mapInfo, err := loader.GetMap(ctx, "program_name", "unhd_hmac_keys")
	// if err != nil { /* handle error */ }

	// Step 4: Create MapLoader for the map
	// mapLoader := NewMapLoader(mapInfo.FD, "unhd_hmac_keys")

	// Step 5: Populate with bpfschema structs
	// In real code, you would use:
	//   type HMACKeyKey struct { FlowID uint16; Namespace uint16; _ [4]byte }
	//   type HMACKeyValue struct { Key [32]byte; CreatedAt uint32; ExpiresAt uint32 }
	//
	//   hmacKeys := map[HMACKeyKey]HMACKeyValue{
	//       {FlowID: 1, Namespace: 1}: {CreatedAt: 100, ExpiresAt: 0},
	//       {FlowID: 2, Namespace: 1}: {CreatedAt: 200, ExpiresAt: 0},
	//   }
	//
	//   entries := make([][2]interface{}, 0, len(hmacKeys))
	//   for k, v := range hmacKeys {
	//       entries = append(entries, [2]interface{}{k, v})
	//   }
	//
	//   if err := mapLoader.LoadMapBatch(entries); err != nil {
	//       // handle error
	//   }

	t.Log("Documentation example demonstrates typical workflow")
}

// TestExamplesCompile verifies that the example code at least compiles
func TestExamplesCompile(t *testing.T) {
	// This test exists only to verify the example code above compiles
	// The ExampleXxx functions above are documentation examples
	t.Log("All example functions compile successfully")
}
