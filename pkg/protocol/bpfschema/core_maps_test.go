// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package bpfschema

import (
	"testing"
	"unsafe"

	"unheaded/pkg/monad"
)

// TestMonadRegisterSize_CoreMaps verifies the Monad register is exactly 20 bytes.
// This MUST match monad-common::Monad in Rust. Mismatch = data corruption.
func TestMonadRegisterSize_CoreMaps(t *testing.T) {
	got := unsafe.Sizeof(MonadRegister{})
	if got != MonadRegisterSize {
		t.Fatalf("MonadRegister size = %d, want %d (must match Rust monad-common::Monad)", got, MonadRegisterSize)
	}
}

// TestAnamnesisEventSize_CoreMaps verifies the event is exactly 32 bytes.
// This MUST match the ring buffer record size in all eBPF programs.
func TestAnamnesisEventSize_CoreMaps(t *testing.T) {
	got := unsafe.Sizeof(AnamnesisEvent{})
	if got != AnamnesisEventSize {
		t.Fatalf("AnamnesisEvent size = %d, want %d", got, AnamnesisEventSize)
	}
}

// TestMbcCpuStateSize_CoreMaps verifies the CPU state is exactly 128 bytes.
func TestMbcCpuStateSize_CoreMaps(t *testing.T) {
	got := unsafe.Sizeof(MbcCpuState{})
	if got != MbcCpuStateSize {
		t.Fatalf("MbcCpuState size = %d, want %d", got, MbcCpuStateSize)
	}
}

// TestFlowKeySize_CoreMaps verifies the 5-tuple key is exactly 16 bytes.
func TestFlowKeySize_CoreMaps(t *testing.T) {
	got := unsafe.Sizeof(FlowKey{})
	if got != FlowKeySize {
		t.Fatalf("FlowKey size = %d, want %d", got, FlowKeySize)
	}
}

// TestCoreMapStructSizes verifies all core map struct sizes.
func TestCoreMapStructSizes(t *testing.T) {
	tests := []struct {
		name string
		size uintptr
		want uintptr
	}{
		{"BlocklistKey", unsafe.Sizeof(BlocklistKey{}), 8},
		{"BlocklistValue", unsafe.Sizeof(BlocklistValue{}), 8},
		{"RateTokenKey", unsafe.Sizeof(RateTokenKey{}), 8},
		{"RateTokenValue", unsafe.Sizeof(RateTokenValue{}), 8},
		{"StatsEntry", unsafe.Sizeof(StatsEntry{}), 8},
		{"ConfigEntry", unsafe.Sizeof(ConfigEntry{}), 8},
		{"SophiaMapKey", unsafe.Sizeof(SophiaMapKey{}), 8},
		{"SophiaMapValue", unsafe.Sizeof(SophiaMapValue{}), 32},
		{"CircuitErrorKey", unsafe.Sizeof(CircuitErrorKey{}), 8},
		{"CircuitErrorValue", unsafe.Sizeof(CircuitErrorValue{}), 8},
		{"FlowState", unsafe.Sizeof(FlowState{}), FlowStateSize},
		{"TraceAssocKey", unsafe.Sizeof(TraceAssocKey{}), 8},
		{"TraceAssocValue", unsafe.Sizeof(TraceAssocValue{}), 16},
		{"ChaosTargetKey", unsafe.Sizeof(ChaosTargetKey{}), 8},
		{"ChaosTargetValue", unsafe.Sizeof(ChaosTargetValue{}), 8},
		{"CacheLineKey", unsafe.Sizeof(CacheLineKey{}), 8},
		{"CacheLineValue", unsafe.Sizeof(CacheLineValue{}), 64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.size != tt.want {
				t.Errorf("%s = %d bytes, want %d", tt.name, tt.size, tt.want)
			}
		})
	}
}

// TestMonadFlagConstants verifies flag bit positions don't overlap.
func TestMonadFlagConstants(t *testing.T) {
	flags := []uint8{
		FlagChaos, FlagCanary, FlagTraced, FlagEncrypt,
		FlagSampled, FlagMirror, FlagCustom, FlagReserved,
	}

	// Each flag should be a single bit.
	for _, f := range flags {
		if f == 0 || (f&(f-1)) != 0 {
			t.Errorf("flag 0x%02x is not a single bit", f)
		}
	}

	// No overlap.
	combined := uint8(0)
	for _, f := range flags {
		if combined&f != 0 {
			t.Errorf("flag 0x%02x overlaps with existing flags 0x%02x", f, combined)
		}
		combined |= f
	}

	// All 8 bits covered.
	if combined != 0xFF {
		t.Errorf("flags don't cover all 8 bits: 0x%02x", combined)
	}
}

// TestEventTypeConstants verifies event types are unique and carry the
// foundation-06 Anamnesis Event Type registry values (E4, 2026-10-26).
func TestEventTypeConstants(t *testing.T) {
	events := map[uint8]string{
		EventBirth:   "BIRTH",
		EventHop:     "HOP",
		EventDeath:   "DEATH",
		EventAnomaly: "ANOMALY",
		EventChaos:   "CHAOS",
	}

	if len(events) != 5 {
		t.Errorf("expected 5 unique event types, got %d", len(events))
	}

	want := map[string]uint8{"BIRTH": 0x01, "HOP": 0x02, "DEATH": 0x03, "CHAOS": 0x04, "ANOMALY": 0x08}
	for name, v := range want {
		if events[v] != name {
			t.Errorf("event type 0x%02x = %q, want %q", v, events[v], name)
		}
	}
	// Registry values with no producer must not be claimed by the schema.
	for _, v := range []uint8{0x00, 0x05, 0x06, 0x07} {
		if _, ok := events[v]; ok {
			t.Errorf("event type 0x%02x (%s) has no producer and must stay unassigned", v, events[v])
		}
	}
}

// TestChaosModeConstants verifies chaos modes are unique.
func TestChaosModeConstants(t *testing.T) {
	modes := map[uint32]string{
		ChaosBitFlip:   "BIT_FLIP",
		ChaosDelay:     "DELAY",
		ChaosDuplicate: "DUPLICATE",
		ChaosTruncate:  "TRUNCATE",
		ChaosMarker:    "MARKER",
	}

	if len(modes) != 5 {
		t.Errorf("expected 5 unique chaos modes, got %d", len(modes))
	}
}

// TestFlagNamesFollowSpec pins bits 1:0 of the flags byte to the
// foundation-06 Flags Bitfield names (CUSTOM 0x02, RSVD 0x01), the same
// names pkg/monad and ebpf/monad-common use. The spec has no 2-bit
// Kingdom Mode field in the flags byte (S-TRUE-WIRE Step 102).
func TestFlagNamesFollowSpec(t *testing.T) {
	if FlagCustom != monad.FlagCustom {
		t.Errorf("FlagCustom = 0x%02x, pkg/monad says 0x%02x", FlagCustom, monad.FlagCustom)
	}
	if FlagReserved != monad.FlagReserved {
		t.Errorf("FlagReserved = 0x%02x, pkg/monad says 0x%02x", FlagReserved, monad.FlagReserved)
	}
	// Deprecated aliases keep their old values for one release.
	if FlagK0 != FlagReserved {
		t.Errorf("FlagK0 = 0x%02x, want alias of FlagReserved 0x%02x", FlagK0, FlagReserved)
	}
	if FlagK1 != FlagCustom {
		t.Errorf("FlagK1 = 0x%02x, want alias of FlagCustom 0x%02x", FlagK1, FlagCustom)
	}
}

// TestActionValues verifies flow action values.
func TestActionValues(t *testing.T) {
	actions := []uint8{ActionForward, ActionTrace, ActionSample, ActionMirror, ActionDrop}
	for i, a := range actions {
		if a != uint8(i+1) {
			t.Errorf("Action[%d] = %d, want %d", i, a, i+1)
		}
	}
}

// TestCircuitStateValues verifies circuit state values.
func TestCircuitStateValues(t *testing.T) {
	states := []uint8{CircuitClosed, CircuitOpen, CircuitHalfOpen}
	for i, s := range states {
		if s != uint8(i+1) {
			t.Errorf("CircuitState[%d] = %d, want %d", i, s, i+1)
		}
	}
}

// TestMapSizeConstants verifies map sizes are reasonable.
func TestMapSizeConstants(t *testing.T) {
	// Ring buffers must be power of 2 (BPF requirement).
	rings := []int{
		AnamnesisRingSize, FlowEventsRingSize, LatencyEventsRingSize,
		PacketEventsRingSize, SyscallEventsRingSize, ComputeEventsRingSize,
	}
	for _, size := range rings {
		if size <= 0 || (size&(size-1)) != 0 {
			t.Errorf("ring buffer size %d is not a power of 2", size)
		}
	}

	// Max entries must be positive.
	entries := []int{
		BlocklistMaxEntries, RateTokensMaxEntries, SophiaMaxEntries,
		FlowsMaxEntries, ChaosTargetsMaxEntries, StatsMaxEntries,
	}
	for _, e := range entries {
		if e <= 0 {
			t.Errorf("max entries %d must be positive", e)
		}
	}
}
