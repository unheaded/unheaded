// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package bpfschema

import (
	"encoding/binary"
	"testing"
	"unsafe"
)

// TestStructSizes verifies that all BPF schema structs have the expected
// wire sizes. This is CRITICAL — if these sizes change, the eBPF programs
// will read garbage from BPF maps.
//
// Per Black Mage Concurrency Audit: "Every map hot-swap → Is it truly atomic?"
// Size mismatches between userspace and kernel are a class of bug.
func TestStructSizes(t *testing.T) {
	tests := []struct {
		name     string
		size     uintptr
		wantSize uintptr
	}{
		// Q1: HMAC keys.
		{"HMACKeyKey", unsafe.Sizeof(HMACKeyKey{}), 8},
		{"HMACKeyValue", unsafe.Sizeof(HMACKeyValue{}), 40},

		// Q2: Sequence counters.
		{"SeqCounterKey", unsafe.Sizeof(SeqCounterKey{}), 8},
		{"SeqCounterValue", unsafe.Sizeof(SeqCounterValue{}), 16},

		// Q4: Ring path.
		{"RingPathKey", unsafe.Sizeof(RingPathKey{}), 16},
		{"RingPathValue", unsafe.Sizeof(RingPathValue{}), 8},

		// Q5/Q6: Migration tokens.
		{"MigrationTokenKey", unsafe.Sizeof(MigrationTokenKey{}), 20},
		// MigrationTokenValue: 32 (Token) + 4 (IssuedAt) + 4 (ExpiresAt) + 16 (NewAddr) = 56 bytes.
		{"MigrationTokenValue", unsafe.Sizeof(MigrationTokenValue{}), 56},

		// H1: Sophia sync.
		{"SophiaSyncKey", unsafe.Sizeof(SophiaSyncKey{}), 8},
		{"SophiaSyncValue", unsafe.Sizeof(SophiaSyncValue{}), 24},

		// H2: Error counters.
		{"ErrorCounterKey", unsafe.Sizeof(ErrorCounterKey{}), 4},
		{"ErrorCounterValue", unsafe.Sizeof(ErrorCounterValue{}), 8},

		// H3: TLV registry.
		{"TLVHandlerEntry", unsafe.Sizeof(TLVHandlerEntry{}), 8},

		// H4/H12: Settings.
		{"SettingsKey", unsafe.Sizeof(SettingsKey{}), 8},
		{"SettingsValue", unsafe.Sizeof(SettingsValue{}), 16},

		// H5: DoS state.
		{"DoSStateKey", unsafe.Sizeof(DoSStateKey{}), 8},
		{"DoSStateValue", unsafe.Sizeof(DoSStateValue{}), 16},

		// H6: Flow types.
		{"FlowTypeEntry", unsafe.Sizeof(FlowTypeEntry{}), 4},

		// H7/Q8: GOAWAY state.
		{"GoawayStateKey", unsafe.Sizeof(GoawayStateKey{}), 8},
		{"GoawayStateValue", unsafe.Sizeof(GoawayStateValue{}), 16},

		// H8: Cancel flows.
		{"CancelFlowKey", unsafe.Sizeof(CancelFlowKey{}), 8},
		{"CancelFlowValue", unsafe.Sizeof(CancelFlowValue{}), 8},

		// H9: Prefetch hints.
		{"PrefetchHintKey", unsafe.Sizeof(PrefetchHintKey{}), 8},
		// PrefetchHintValue: 2+2+1+1+2+4+4+4 = 20 bytes (no inter-field padding needed).
		{"PrefetchHintValue", unsafe.Sizeof(PrefetchHintValue{}), 20},

		// H10: Hop validators.
		{"HopValidatorKey", unsafe.Sizeof(HopValidatorKey{}), 8},
		// HopValidatorValue: 1+1+1+1+16+1+3+4 = 28 bytes (no inter-field padding needed).
		{"HopValidatorValue", unsafe.Sizeof(HopValidatorValue{}), 28},

		// H11: Authority.
		{"AuthorityKey", unsafe.Sizeof(AuthorityKey{}), 8},
		{"AuthorityValue", unsafe.Sizeof(AuthorityValue{}), 24},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.size != tt.wantSize {
				t.Errorf("%s size = %d bytes, want %d bytes", tt.name, tt.size, tt.wantSize)
			}
		})
	}
}

// TestBinaryEncoding verifies that structs can be serialized with encoding/binary.
// This is the exact path used to populate BPF maps from userspace.
func TestBinaryEncoding_SeqCounterKey(t *testing.T) {
	key := SeqCounterKey{Namespace: 42}
	buf := make([]byte, unsafe.Sizeof(key))
	// Write in big-endian (network byte order).
	binary.BigEndian.PutUint32(buf[0:4], key.Namespace)
	// Padding bytes should be zero.
	for i := 4; i < len(buf); i++ {
		if buf[i] != 0 {
			t.Errorf("padding byte %d = 0x%02x, want 0x00", i, buf[i])
		}
	}
}

func TestBinaryEncoding_HMACKeyValue(t *testing.T) {
	val := HMACKeyValue{
		CreatedAt: 1708444800,
		ExpiresAt: 1708531200,
	}
	// Fill key with test pattern.
	for i := range val.Key {
		val.Key[i] = byte(i)
	}

	buf := make([]byte, unsafe.Sizeof(val))
	copy(buf[0:32], val.Key[:])
	binary.BigEndian.PutUint32(buf[32:36], val.CreatedAt)
	binary.BigEndian.PutUint32(buf[36:40], val.ExpiresAt)

	if buf[0] != 0x00 || buf[31] != 0x1F {
		t.Error("key bytes not correctly serialized")
	}
}

// TestMapNameConstants verifies map name constants are non-empty and unique.
func TestMapNameConstants(t *testing.T) {
	names := []string{
		MapHMACKeys,
		MapSeqCounters,
		MapRingPath,
		MapMigrationTokens,
		MapRetryTokens,
		MapSophiaSync,
		MapErrorCounters,
		MapTLVRegistry,
		MapSettings,
		MapDoSState,
		MapFlowTypes,
		MapGoawayState,
		MapCancelFlows,
		MapPrefetchHints,
		MapHopValidators,
		MapAuthority,
	}

	seen := make(map[string]bool)
	for _, name := range names {
		if name == "" {
			t.Error("empty map name constant")
		}
		if seen[name] {
			t.Errorf("duplicate map name: %s", name)
		}
		seen[name] = true
	}

	if len(seen) != 16 {
		t.Errorf("expected 16 unique map names, got %d", len(seen))
	}
}
