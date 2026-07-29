// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package bpfschema defines the canonical BPF map key/value structures shared
// between Go userspace (map population, reading) and eBPF kernel programs
// (map lookup, update).
//
// CRITICAL: These structs MUST be kept in sync with the corresponding Rust
// definitions in ebpf/monad-common/src/lib.rs. Any change here requires a
// corresponding change there. The PATTERN_MATRIX.md documents which structs
// map to which BPF maps.
//
// All structs use fixed-size fields compatible with encoding/binary.
// All multi-byte fields are big-endian (network byte order) per RFC 8200.
// No padding is added — struct size == wire size.
//
// Map type selection follows RFC 9669 §5.4 BPF map semantics:
//   - HASH: Exact-match lookup, dynamic sizing
//   - PERCPU_HASH: Per-CPU counters, no lock contention
//   - PERCPU_ARRAY: Fixed-index per-CPU values
//   - LRU_HASH: Bounded memory with automatic eviction
//   - ARRAY: Fixed-size, indexed access
//
// See: Black Mage Concurrency Audit Checklist for map access patterns.
package bpfschema

// MapName constants define the pinned BPF map names.
// These correspond 1:1 with the Pattern Matrix entries.
const (
	// Q1: HMAC keys for per-flow replay protection (RFC 9000 §8.1).
	MapHMACKeys = "unhd_hmac_keys"

	// Q2: Per-namespace monotonic sequence counters (RFC 9000 §5.1.1).
	MapSeqCounters = "unhd_seq_counters"

	// Q4: Ring path counters for amplification limiting (RFC 9000 §21.3).
	MapRingPath = "unhd_ring_path"

	// Q5: Flow migration validation tokens (RFC 9000 §9).
	MapMigrationTokens = "unhd_migration_tokens" // #nosec G101 -- BPF map name, not a token value

	// Q6: Shield retry tokens with HMAC validation (RFC 9000 §8.1.2).
	MapRetryTokens = "unhd_retry_tokens" // #nosec G101 -- BPF map name, not a token value

	// H1: Sophia dictionary encoder/decoder ACK state (RFC 9114 §4.1).
	MapSophiaSync = "unhd_sophia_sync"

	// H2: Error code counters per type (RFC 9114 §8).
	MapErrorCounters = "unhd_error_counters"

	// H3: TLV type → handler dispatch table (RFC 9114 §9).
	MapTLVRegistry = "unhd_tlv_registry"

	// H4/H12: Per-connection capability settings (RFC 9114 §7.2.4).
	MapSettings = "unhd_settings"

	// H5: DoS backpressure state tracking (RFC 9114 §10).
	MapDoSState = "unhd_dos_state"

	// H6: Flow type classification table (RFC 9114 §6).
	MapFlowTypes = "unhd_flow_types"

	// H7/Q8: GOAWAY and stateless reset state (RFC 9114 §7.2.6).
	MapGoawayState = "unhd_goaway_state"

	// H8: Per-flow cancellation state (RFC 9114 §4.1.1).
	MapCancelFlows = "unhd_cancel_flows"

	// H9: Explicit prefetch hint tracking (RFC 7240 §2).
	MapPrefetchHints = "unhd_prefetch_hints"

	// H10: Per-hop packet validators (RFC 8200 §4).
	MapHopValidators = "unhd_hop_validators"

	// H11: Dictionary authority enforcement (RFC 8200 §4.2).
	MapAuthority = "unhd_authority"
)

// === Q1: HMAC Key Entries (integrity package) ===

// HMACKeyKey is the lookup key for per-flow HMAC keys.
// Size: 8 bytes. Map type: HASH.
type HMACKeyKey struct {
	FlowID    uint16  // Flow identifier from Monad header.
	Namespace uint16  // Namespace ID.
	_         [4]byte // Reserved, must be zero.
}

// HMACKeyValue stores the HMAC-SHA256 key material.
// Size: 40 bytes.
type HMACKeyValue struct {
	Key       [32]byte // HMAC-SHA256 key (256 bits).
	CreatedAt uint32   // Unix timestamp of key creation.
	ExpiresAt uint32   // Unix timestamp of key expiry. 0 = no expiry.
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

// === Q4: Ring Path Counters (amplification package) ===

// RingPathKey identifies a flow for amplification tracking.
// Size: 16 bytes. Map type: HASH.
type RingPathKey struct {
	SrcAddr [16]byte // IPv6 source address.
}

// RingPathValue tracks ring path state.
// Size: 8 bytes.
type RingPathValue struct {
	HopCount  uint16 // Current ring hop count.
	MaxHops   uint16 // Configured maximum hops.
	BytesSent uint32 // Bytes sent (for 3× ratio enforcement).
}

// === Q5/Q6: Migration & Retry Tokens (migration package) ===

// MigrationTokenKey identifies a flow migration.
// Size: 20 bytes. Map type: LRU_HASH.
type MigrationTokenKey struct {
	FlowID  uint16   // Flow identifier.
	SrcAddr [16]byte // Original source IPv6 address.
	_       [2]byte
}

// MigrationTokenValue stores the HMAC-signed token.
// Size: 56 bytes (32 Token + 4 IssuedAt + 4 ExpiresAt + 16 NewAddr).
type MigrationTokenValue struct {
	Token     [32]byte // HMAC-SHA256 token.
	IssuedAt  uint32   // Unix timestamp.
	ExpiresAt uint32   // Unix timestamp.
	NewAddr   [16]byte // Destination (new) IPv6 address.
}

// === H1: Sophia Sync State (sophiasync package) ===

// SophiaSyncKey identifies a dictionary sync channel.
// Size: 8 bytes. Map type: HASH.
type SophiaSyncKey struct {
	DictID uint16 // Dictionary identifier.
	PeerID uint16 // Peer node identifier.
	_      [4]byte
}

// SophiaSyncValue tracks encoder/decoder ACK state.
// Size: 24 bytes.
type SophiaSyncValue struct {
	TableVersion uint32  // Current table version.
	AckedVersion uint32  // Last ACK'd version by peer.
	PendingDelta uint32  // Pending delta entries count.
	LastSyncAt   uint32  // Unix timestamp of last sync.
	Signature    [8]byte // Truncated HMAC of current state.
}

// === H2: Error Counters (errors package) ===

// ErrorCounterKey indexes by error code.
// Size: 4 bytes. Map type: PERCPU_ARRAY (index = error code).
type ErrorCounterKey struct {
	Code uint16 // Error code (0x0000-0x00FF).
	_    [2]byte
}

// ErrorCounterValue is a per-CPU counter.
// Size: 8 bytes.
type ErrorCounterValue struct {
	Count      uint32 // Total occurrences.
	LastSeenAt uint32 // Unix timestamp of last occurrence.
}

// === H3: TLV Registry (tlv package) ===

// TLVHandlerEntry maps TLV types to handler programs.
// Size: 8 bytes. Map type: ARRAY (index = TLV type).
type TLVHandlerEntry struct {
	ProgFD   uint32 // File descriptor of handler BPF program.
	Flags    uint16 // Handler flags (0x01 = required, 0x02 = mutable).
	Reserved uint16 // Must be zero.
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

// === H7/Q8: GOAWAY State (lifecycle package) ===

// GoawayStateKey identifies a connection's GOAWAY state.
// Size: 8 bytes. Map type: HASH.
type GoawayStateKey struct {
	ConnID uint32 // Connection identifier.
	_      [4]byte
}

// GoawayStateValue tracks GOAWAY monotonicity.
// Size: 16 bytes.
type GoawayStateValue struct {
	LastFlowID uint32 // Last-flow-ID from most recent GOAWAY.
	SentAt     uint32 // Unix timestamp of GOAWAY send.
	Reason     uint16 // Error code accompanying GOAWAY.
	Count      uint16 // Number of GOAWAYs sent (for graceful shutdown).
	Flags      uint32 // 0x01 = sent, 0x02 = received, 0x04 = draining.
}

// === H8: Cancel Flows (lifecycle package) ===

// CancelFlowKey identifies a cancelled flow.
// Size: 8 bytes. Map type: LRU_HASH.
// Field order: largest-first to avoid Go alignment padding.
type CancelFlowKey struct {
	ConnID uint32 // Connection identifier.
	FlowID uint16 // Flow identifier.
	_      [2]byte
}

// CancelFlowValue stores cancellation metadata.
// Size: 8 bytes.
// Field order: largest-first to avoid Go alignment padding.
type CancelFlowValue struct {
	CancelledAt uint32 // Unix timestamp.
	ErrorCode   uint16 // Error code for cancellation reason.
	_           [2]byte
}

// === H9: Prefetch Hints (prefetch package) ===

// PrefetchHintKey identifies a prefetch hint.
// Size: 8 bytes. Map type: LRU_HASH.
type PrefetchHintKey struct {
	FlowID uint16 // Flow identifier.
	HintID uint16 // Hint sequence number.
	ConnID uint32 // Connection identifier.
}

// PrefetchHintValue stores hint metadata.
// Size: 20 bytes (2+2+1+1+2+4+4+4).
type PrefetchHintValue struct {
	DictID    uint16 // Dictionary to prefetch.
	FieldID   uint16 // Field within dictionary.
	Priority  uint8  // 0-3 priority level.
	Status    uint8  // 0=pending, 1=fulfilled, 2=cancelled.
	_         [2]byte
	IssuedAt  uint32 // Unix timestamp.
	ExpiresAt uint32 // Unix timestamp.
	_         [4]byte
}

// === H10: Hop Validators (intermediary package) ===

// HopValidatorKey identifies a hop in the path.
// Size: 8 bytes. Map type: HASH.
type HopValidatorKey struct {
	HopID     uint32 // Hop identifier (position in ring).
	Namespace uint32 // Namespace.
}

// HopValidatorValue defines validation rules for a hop.
// Size: 28 bytes (1+1+1+1+16+1+3+4).
type HopValidatorValue struct {
	AllowedVersionMin uint8     // Minimum accepted Monad version.
	AllowedVersionMax uint8     // Maximum accepted Monad version.
	RequireCRC        uint8     // 0x01 = CRC validation required.
	RequireHMAC       uint8     // 0x01 = HMAC validation required.
	AllowedDictIDs    [8]uint16 // Up to 8 allowed dictionary IDs. 0 = unused.
	MaxHopCount       uint8     // Maximum allowed hop count.
	_                 [3]byte
	Flags             uint32 // Validation flags.
}

// === H11: Authority (intermediary package) ===

// AuthorityKey identifies a dictionary authority binding.
// Size: 8 bytes. Map type: HASH.
type AuthorityKey struct {
	DictID    uint16 // Dictionary identifier.
	Namespace uint16 // Namespace.
	_         [4]byte
}

// AuthorityValue stores authority metadata.
// Size: 24 bytes.
type AuthorityValue struct {
	OwnerFingerprint [16]byte // Truncated SHA-256 of owner's public key.
	GrantedAt        uint32   // Unix timestamp.
	ExpiresAt        uint32   // Unix timestamp. 0 = no expiry.
}
