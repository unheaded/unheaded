// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package bpfschema

// Map-name constants are the names the Rust eBPF programs give their maps:
// Aya names a #[map] by its static identifier, and that is the key in a
// cilium/ebpf CollectionSpec (the kernel truncates names to 15 bytes in
// bpftool output). rust_source_test.go fails if a constant names no map
// declared under ebpf/*/src. The old lowercase "unhd_*" values named no
// loaded map; the Q4 ring path, H1 Sophia sync, H3 TLV registry and H9
// prefetch hint maps have no Rust program at all, so their names are gone
// (S-TRUE-WIRE Step 118).
const (
	// Q1: HMAC keys, read for presence and expiry only (hop-ebpf).
	MapHMACKeys = "HMAC_KEYS"

	// Q2: Per-namespace monotonic sequence counters (hop-ebpf).
	MapSeqCounters = "SEQ_COUNTERS"

	// Q5: Flow migration validation tokens (flow-tracker).
	MapMigrationTokens = "MIGRATION_TOKENS" // #nosec G101 -- BPF map name, not a token value

	// Q6: Validated sources; no retry token is on the wire (hop-ebpf).
	MapRetryTokens = "RETRY_TOKENS" // #nosec G101 -- BPF map name, not a token value

	// H2: Error code counters (shield-ebpf).
	MapErrorCounters = "ERROR_COUNTERS"

	// H4/H12: Per-connection capability settings (hop-ebpf).
	MapSettings = "SETTINGS"

	// H5: DoS backpressure state tracking (hop-ebpf).
	MapDoSState = "DOS_STATE"

	// H6: Flow type classification table (hop-ebpf).
	MapFlowTypes = "FLOW_TYPES"

	// H8: Per-flow cancellation state (flow-tracker).
	MapCancelFlows = "CANCEL_FLOWS"

	// H10: Per-hop packet validators (hop-ebpf).
	MapHopValidators = "HOP_VALIDATORS"

	// Per-thread RTT statistics keyed by trace id, pid, tid (latency-probe).
	MapLatency = "LATENCY_MAP"
)

// === Q1: HMAC Key Entries (integrity package) ===

// HMACKeyKey is the lookup key for per-flow HMAC keys.
// Size: 8 bytes. Map type: HASH.
type HMACKeyKey struct {
	FlowID    uint16  // src_service_id | dst_service_id << 8, not a flow label.
	Namespace uint16  // Namespace ID.
	_         [4]byte // Reserved, must be zero.
}

// HMACKeyValue stores the HMAC-SHA256 key material: hop-ebpf's HmacKeyEntry.
// Size: 40 bytes.
type HMACKeyValue struct {
	Key       [32]byte // HMAC-SHA256 key (256 bits).
	CreatedAt uint32   // CLOCK_BOOTTIME seconds.
	ExpiresAt uint32   // CLOCK_BOOTTIME seconds, 0 = never.
}

// === Q2: Sequence Counters (sequence package) ===

// SeqCounterKey identifies a namespace for sequence tracking.
// Size: 8 bytes. Map type: PERCPU_HASH.
type SeqCounterKey struct {
	Namespace uint32 // Namespace identifier.
	_         [4]byte
}

// SeqCounterValue holds the current sequence state.
// Size: 16 bytes.
type SeqCounterValue struct {
	Current   uint32 // Current sequence number.
	Highest   uint32 // Highest seen (for gap detection).
	Gaps      uint32 // Total gaps detected.
	Reordered uint32 // Total reordering events.
}

// === Q5/Q6: Migration & Retry Tokens (migration package) ===

// MIGRATION_TOKENS is keyed by FlowKey with a FlowMigrationTokenValue
// (core_maps.go), the flow-tracker layout. The old MigrationTokenKey (20 B)
// and MigrationTokenValue (56 B) matched no Rust map and were removed.

// RetryTokenKey is the key of hop-ebpf RETRY_TOKENS: make_retry_key's
// flow_id | src_low48 << 16 as a little-endian u64. src_low48 is a number,
// the last six source address bytes read big-endian, so those bytes sit here
// in reverse order: fd00::1 is {0x01, 0, 0, 0, 0, 0}.
// Size: 8 bytes. Map type: HASH.
type RetryTokenKey struct {
	FlowID   uint16  // src_service_id | dst_service_id << 8, not a flow label.
	SrcLow48 [6]byte // Least significant byte first.
}

// RetryTokenValue is a source validated earlier: hop-ebpf's RetryEntry. No
// retry token is on the wire, so Token is never compared.
// Size: 48 bytes.
type RetryTokenValue struct {
	Token     [32]byte
	IssuedAt  uint32  // CLOCK_BOOTTIME seconds.
	ExpiresAt uint32  // CLOCK_BOOTTIME seconds, 0 = never.
	_         [8]byte // Reserved, must be zero.
}

// === H2: Error Counters (errors package) ===

// ErrorCounterKey indexes by error code.
// Size: 8 bytes, the u64 key of shield-ebpf ERROR_COUNTERS (HashMap<u64, [u8; 8]>).
type ErrorCounterKey struct {
	Code uint16 // Error code (0x0000-0x00FF).
	_    [6]byte
}

// ErrorCounterValue is a per-CPU counter.
// Size: 8 bytes.
type ErrorCounterValue struct {
	Count      uint32 // Total occurrences.
	LastSeenAt uint32 // Unix timestamp of last occurrence.
}

// === H4/H12: Settings (settings package) ===

// SettingsKey identifies a setting by connection + setting ID.
// Size: 8 bytes. Map type: HASH.
type SettingsKey struct {
	ConnID    uint32 // Connection identifier.
	SettingID uint16 // Setting identifier (varint-encoded ID).
	_         [2]byte
}

// SettingsValue stores a negotiated setting value.
// Size: 16 bytes.
type SettingsValue struct {
	Value        uint64 // Setting value (varint-decoded).
	NegotiatedAt uint32 // Unix timestamp.
	Flags        uint32 // 0x01 = peer-set, 0x02 = local-set, 0x04 = default.
}

// === H5: DoS State (dos package) ===

// DoSStateKey identifies a backpressure monitoring point.
// Size: 8 bytes. Map type: PERCPU_HASH.
type DoSStateKey struct {
	Namespace uint32 // Namespace identifier.
	_         [4]byte
}

// DoSStateValue tracks drop rate and backpressure level.
// Size: 16 bytes.
type DoSStateValue struct {
	DropCount       uint32 // Drops in current window.
	TotalCount      uint32 // Total packets in current window.
	BackpressureLvl uint8  // 0=none, 1=low, 2=med, 3=high, 4=critical.
	_               [3]byte
	WindowStart     uint32 // Unix timestamp of window start.
}

// === H6: Flow Types (flowtype package) ===

// FlowTypeEntry classifies a flow.
// Size: 4 bytes. Map type: ARRAY (index = flow_id).
type FlowTypeEntry struct {
	Type  uint8 // 0x00=control, 0x01=data, 0x02=prefetch, 0x03=reserved.
	Flags uint8 // Per-type flags.
	_     [2]byte
}

// H7/Q8 (GOAWAY state) was removed with shield-ebpf's GOAWAY_STATE map
// (S-TRUE-WIRE Step 207): no current draft defines GOAWAY on Kingdom streams.

// === H8: Cancel Flows (lifecycle package) ===

// CANCEL_FLOWS is keyed by FlowKey with a FlowCancelValue (core_maps.go),
// the flow-tracker layout. The old CancelFlowKey and CancelFlowValue
// (8 B each) matched no Rust map and were removed.

// === H10: Hop Validators (intermediary package) ===

// HopValidatorKey identifies a hop in the path.
// Size: 8 bytes. Map type: HASH.
type HopValidatorKey struct {
	HopID     uint32 // Hop identifier (position in ring).
	Namespace uint32 // Namespace.
}

// HopValidatorValue defines validation rules for a hop.
// Size: 32 bytes (1+1+1+1+16+1+3+4+4), hop-ebpf's HopValidator. The trailing
// 4 bytes are reserved.
type HopValidatorValue struct {
	AllowedVersionMin uint8     // Minimum accepted Monad version.
	AllowedVersionMax uint8     // Maximum accepted Monad version.
	RequireCRC        uint8     // 0x01 = CRC validation required.
	RequireHMAC       uint8     // 0x01 = HMAC validation required.
	AllowedDictIDs    [8]uint16 // Up to 8 allowed dictionary IDs. 0 = unused.
	MaxHopCount       uint8     // Maximum allowed hop count.
	_                 [3]byte
	Flags             uint32  // Validation flags.
	_                 [4]byte // Reserved, must be zero (Rust value is 32 bytes).
}
