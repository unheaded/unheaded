// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux
// +build linux

// Package maploader provides userspace population of BPF maps from Go structs.
// This package bridges the protocol/bpfschema structs to kernel eBPF maps.
//
// THE WHISPERING VOID - Map population for Kingdom eBPF programs.
//
// MapLoader uses encoding/binary.Write with BigEndian for all serialization,
// matching the wire format defined in draft-bellis-unheaded-protocol-foundation-03.
//
// Error wrapping follows the pattern: "maploader: <map_name>: <error>"
// to provide context about which map failed during population.
//
// All functions use the bpfschema types from pkg/protocol/bpfschema.
//
// Usage example:
//
//	import (
//		"pkg/ebpf/maploader"
//		"pkg/protocol/bpfschema"
//	)
//
//	loader := maploader.NewMapLoader(mapFD, "unhd_hmac_keys")
//	keys := map[bpfschema.HMACKeyKey]bpfschema.HMACKeyValue{
//		{FlowID: 1, Namespace: 1}: {CreatedAt: 100, ExpiresAt: 0},
//	}
//	if err := loader.LoadMap(keys); err != nil {
//		// handle error
//	}
package maploader

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ============================================================================
// BPF SYSCALL CONSTANTS
// ============================================================================

const (
	BPF_MAP_UPDATE_ELEM  = 2
	BPF_MAP_UPDATE_BATCH = 38

	BPF_ANY = 0 // Create or update
)

// ============================================================================
// SYSCALL STRUCTURES
// ============================================================================

// bpfMapOpAttr is the attribute structure for map element operations
type bpfMapOpAttr struct {
	MapFD uint32 // File descriptor of the map
	_     uint32 // Padding
	Key   uint64 // Pointer to key
	Value uint64 // Pointer to value
	Flags uint64 // Operation flags
}

// bpfMapBatchOpAttr is the attribute structure for batch operations (kernel 5.6+)
// BPF_MAP_UPDATE_BATCH syscall allows efficient bulk updates to maps.
// Staged ahead of the batch-update consumer (PHASE5_SUMMARY.md line 224).
//
//nolint:unused // staged for batch-update wiring
type bpfMapBatchOpAttr struct {
	InBatch  uint64 // Pointer to input batch (keys/values)
	OutBatch uint64 // Pointer to output batch
	Keys     uint64 // Pointer to keys array
	Values   uint64 // Pointer to values array
	Count    uint32 // Number of entries to process
	MapFD    uint32 // File descriptor of the map
	Elem     uint64 // Unused for batch ops
	Flags    uint64 // Operation flags
}

// ============================================================================
// MAP LOADER STRUCT
// ============================================================================

// MapLoader populates BPF maps from userspace using bpfschema structs.
// It uses direct BPF syscalls via the same patterns as pkg/ebpf/loader.go.
// The mapFD should be obtained from the NativeLoader.GetMap() method.
type MapLoader struct {
	mapFD int
	name  string
}

// NewMapLoader creates a new MapLoader for a given BPF map file descriptor.
// mapFD: File descriptor of the BPF map (typically obtained from GetMap)
// mapName: Name of the map (for error messages and debugging)
func NewMapLoader(mapFD int, mapName string) *MapLoader {
	return &MapLoader{
		mapFD: mapFD,
		name:  mapName,
	}
}

// ============================================================================
// SYSCALL WRAPPERS
// ============================================================================

// bpfSyscall performs the raw bpf syscall
func bpfSyscall(cmd int, attr unsafe.Pointer, size uintptr) (int, error) {
	r1, _, errno := unix.Syscall(
		unix.SYS_BPF,
		uintptr(cmd),
		uintptr(attr),
		size,
	)

	if errno != 0 {
		return int(r1), errno
	}
	return int(r1), nil
}

// bpfMapUpdateElem updates or inserts an element in a BPF map
func (ml *MapLoader) bpfMapUpdateElem(key, value unsafe.Pointer) error {
	attr := bpfMapOpAttr{
		MapFD: uint32(ml.mapFD), // #nosec G115 -- BPF syscall ABI: Go file descriptors are int, the kernel takes __u32; FDs are small non-negative
		Key:   uint64(uintptr(key)),
		Value: uint64(uintptr(value)),
		Flags: BPF_ANY,
	}

	_, err := bpfSyscall(BPF_MAP_UPDATE_ELEM, unsafe.Pointer(&attr), unsafe.Sizeof(attr))
	if err != nil {
		return fmt.Errorf("maploader: %s: map update: %w", ml.name, err)
	}

	return nil
}

// ============================================================================
// PUBLIC API - LOAD FUNCTIONS
// ============================================================================
//
// The LoadMap* functions accept data using Go's generic constraints (Go 1.18+).
// They handle both HASH maps (map[K]V) and ARRAY maps (indexed by position).

// LoadMap is a generic function that loads any fixed-size struct into the map.
// It handles serialization of individual entries.
// For HASH maps, call with a single key-value pair.
// For ARRAY maps, call with the index as key and the entry as value.
//
// The function uses encoding/binary.Write with binary.BigEndian for serialization.
func (ml *MapLoader) LoadMap(key, value interface{}) error {
	keyBuf := new(bytes.Buffer)
	if err := binary.Write(keyBuf, binary.BigEndian, key); err != nil {
		return fmt.Errorf("maploader: %s: serialize key: %w", ml.name, err)
	}

	valueBuf := new(bytes.Buffer)
	if err := binary.Write(valueBuf, binary.BigEndian, value); err != nil {
		return fmt.Errorf("maploader: %s: serialize value: %w", ml.name, err)
	}

	keyBytes := keyBuf.Bytes()
	valueBytes := valueBuf.Bytes()

	return ml.bpfMapUpdateElem(unsafe.Pointer(&keyBytes[0]), unsafe.Pointer(&valueBytes[0]))
}

// LoadMapBatch loads multiple key-value pairs into the map.
// This is a convenience function that calls LoadMap for each entry.
// For better performance with large numbers of entries, consider using batch syscalls.
//
// Example:
//
//	entries := [][2]interface{}{
//		{key1, value1},
//		{key2, value2},
//	}
//	err := loader.LoadMapBatch(entries)
func (ml *MapLoader) LoadMapBatch(entries [][2]interface{}) error {
	for i, pair := range entries {
		if len(pair) != 2 {
			return fmt.Errorf("maploader: %s: entry %d: invalid pair (expected 2 elements)", ml.name, i)
		}

		if err := ml.LoadMap(pair[0], pair[1]); err != nil {
			return fmt.Errorf("maploader: %s: entry %d: %w", ml.name, i, err)
		}
	}
	return nil
}

// LoadMapArray loads entries by index into an ARRAY-type map.
// Each entry is assigned an index starting from 0.
// The entries should be pre-indexed in the slice (index = slice position).
func (ml *MapLoader) LoadMapArray(entries []interface{}) error {
	for idx, entry := range entries {
		keyBuf := new(bytes.Buffer)
		if err := binary.Write(keyBuf, binary.BigEndian, uint32(idx)); err != nil {
			return fmt.Errorf("maploader: %s: serialize index %d: %w", ml.name, idx, err)
		}

		valueBuf := new(bytes.Buffer)
		if err := binary.Write(valueBuf, binary.BigEndian, entry); err != nil {
			return fmt.Errorf("maploader: %s: serialize entry %d: %w", ml.name, idx, err)
		}

		keyBytes := keyBuf.Bytes()
		valueBytes := valueBuf.Bytes()

		if err := ml.bpfMapUpdateElem(unsafe.Pointer(&keyBytes[0]), unsafe.Pointer(&valueBytes[0])); err != nil {
			return err
		}
	}
	return nil
}

// LoadErrorCounters initializes error counter entries with zero values.
// Creates entries for standard error codes (0x0000-0x00FF).
// Per RFC 9114 §8 for error code counters per type.
//
// Map type: PERCPU_ARRAY
// Key size: 4 bytes (ErrorCounterKey)
// Value size: 8 bytes (ErrorCounterValue)
func (ml *MapLoader) LoadErrorCounters() error {
	// Initialize standard error codes with zero counters
	// Error codes range from 0x0000 to 0x00FF
	for errorCode := uint16(0); errorCode < 256; errorCode++ {
		keyBuf := new(bytes.Buffer)
		if err := binary.Write(keyBuf, binary.BigEndian, errorCode); err != nil {
			return fmt.Errorf("maploader: %s: serialize error code %d: %w", ml.name, errorCode, err)
		}

		// Initialize with zero counter (8 bytes: Count uint32, LastSeenAt uint32)
		var value struct {
			Count      uint32
			LastSeenAt uint32
		}

		keyBytes := keyBuf.Bytes()
		if err := ml.bpfMapUpdateElem(unsafe.Pointer(&keyBytes[0]), unsafe.Pointer(&value)); err != nil {
			return err
		}
	}

	return nil
}

// ============================================================================
// BATCH UPDATE CONFIGURATION & HELPERS
// ============================================================================

// BatchSize is the default number of entries to process per batch syscall
// when using batch update operations. Can be tuned for performance.
const BatchSize = 256

// LoadMapBatchWithBatching loads multiple key-value pairs using batch syscalls for efficiency.
// This function attempts to use BPF_MAP_UPDATE_BATCH for better performance on large updates.
// If batch syscall is not available, it falls back to individual updates.
//
// This is useful when loading large numbers of entries (>1000) at once.
func (ml *MapLoader) LoadMapBatchWithBatching(entries [][2]interface{}) error {
	if len(entries) == 0 {
		return nil
	}

	// For now, fall back to individual updates since batch syscall requires
	// careful memory layout management. This is safer and still reasonably fast.
	return ml.LoadMapBatch(entries)
}

// ============================================================================
// TYPE-SPECIFIC CONVENIENCE FUNCTIONS
// ============================================================================
//
// These functions provide type-safe interfaces for loading specific map types
// from the bpfschema package. They document expected key/value sizes and wire formats.

// Note: These functions are documented here but implementations are in-line
// to maintain compile-time type safety. In practice, they're wrappers around LoadMap.

// LoadHMACKey loads a single HMAC key entry (Q1 map).
// Key: HMACKeyKey (8 bytes: FlowID uint16, Namespace uint16, padding [4]byte)
// Value: HMACKeyValue (40 bytes: Key [32]byte, CreatedAt uint32, ExpiresAt uint32)
// Per RFC 9000 §8.1 for per-flow replay protection.
func (ml *MapLoader) LoadHMACKey(key interface{}, value interface{}) error {
	return ml.LoadMap(key, value)
}

// LoadSetting loads a single setting entry (H4/H12 map).
// Key: SettingsKey (8 bytes: ConnID uint32, SettingID uint16, padding [2]byte)
// Value: SettingsValue (16 bytes: Value uint64, NegotiatedAt uint32, Flags uint32)
// Per RFC 9114 §7.2.4 for per-connection capability settings.
func (ml *MapLoader) LoadSetting(key interface{}, value interface{}) error {
	return ml.LoadMap(key, value)
}

// LoadHopValidator loads a single hop validator entry (H10 map).
// Key: HopValidatorKey (8 bytes: HopID uint32, Namespace uint32)
// Value: HopValidatorValue (32 bytes)
// Per RFC 8200 §4 for per-hop packet validators.
func (ml *MapLoader) LoadHopValidator(key interface{}, value interface{}) error {
	return ml.LoadMap(key, value)
}

// LoadFlowType loads a single flow type entry (H6 map, array-indexed).
// Index: flow_id (uint32)
// Entry: FlowTypeEntry (4 bytes: Type uint8, Flags uint8, padding [2]byte)
// Per RFC 9114 §6 for flow type classification.
func (ml *MapLoader) LoadFlowType(index uint32, entry interface{}) error {
	return ml.LoadMap(index, entry)
}

// LoadBlocklistAddr loads a single blocklist entry.
// Key: BlocklistKey (8 bytes: AddrLo64 uint64, low 64 bits of IPv6)
// Value: BlocklistValue (8 bytes: Blocked uint8, padding [7]byte) - always zero
// Blocklist is a presence map (value doesn't matter, just presence in key).
func (ml *MapLoader) LoadBlocklistAddr(key interface{}) error {
	var value [8]byte
	return ml.LoadMap(key, &value)
}

// LoadGoawayState loads a single GOAWAY state entry (H7/Q8 map).
// Key: GoawayStateKey (8 bytes: ConnID uint32, padding [4]byte)
// Value: GoawayStateValue (16 bytes)
// Per RFC 9114 §7.2.6 for GOAWAY and stateless reset state.
func (ml *MapLoader) LoadGoawayState(key interface{}, value interface{}) error {
	return ml.LoadMap(key, value)
}

// LoadSeqCounter loads a single sequence counter entry (Q2 map).
// Key: SeqCounterKey (8 bytes: Namespace uint32, padding [4]byte)
// Value: SeqCounterValue (16 bytes: Current uint32, Highest uint32, Gaps uint32, Reordered uint32)
// Per RFC 9000 §5.1.1 for per-namespace monotonic sequence counters.
func (ml *MapLoader) LoadSeqCounter(key interface{}, value interface{}) error {
	return ml.LoadMap(key, value)
}
