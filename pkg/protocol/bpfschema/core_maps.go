// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package bpfschema — core_maps.go
//
// Defines BPF map key/value structs for the EXISTING eBPF programs
// (Shield, Hop, Yaldabaoth, Flow Tracker, Monad CPU, Latency Probe,
// Packet Marker, Syscall Tracer).
//
// These structs MUST match the Rust definitions in:
//   - ebpf/monad-common/src/lib.rs (Monad, AnamnesisEvent, CrcCcitt)
//   - ebpf/shield-ebpf/src/main.rs (BLOCKLIST, RATE_TOKENS, STATS)
//   - ebpf/hop-ebpf/src/main.rs (SOPHIA, CIRCUIT_ERRORS, CONFIG)
//   - ebpf/flow-tracker/src/main.rs (FlowKey, FlowState, TraceId)
//   - ebpf/monad-cpu-ebpf/src/main.rs (MbcCpuState, ComputeHopEvent)
//
// Wire format per draft-bellis-unheaded-protocol-foundation-03 §3.
// All multi-byte fields big-endian per RFC 8200.
package bpfschema

// === MONAD REGISTER (20 bytes) ===
// Per draft-bellis-unheaded-protocol-foundation-03 §3.
// Carried in IPv6 Hop-by-Hop Options header (RFC 8200 §4.2).

// MonadRegister is the 20-byte Monad metadata register.
// This is the CANONICAL Go representation matching monad-common::Monad.
// Rust source: ebpf/monad-common/src/lib.rs:301-379
// Per draft-bellis-unheaded-protocol-foundation-01 §3.3
//
// Wire layout (20 bytes):
//
//	Offset  Field           Size    Encoding
//	0x00    Version         1B      Raw u8, MUST be 0x01
//	0x01    SrcServiceID    1B      Exponent-encoded (Sophia dict 0x03)
//	0x02    DstServiceID    1B      Exponent-encoded (Sophia dict 0x03)
//	0x03    HopCount        1B      Raw u8, incremented per hop, saturates at 255
//	0x04    QoSClass        1B      Exponent-encoded (Sophia dict 0x01)
//	0x05    FlowAction      1B      Exponent: 1=FORWARD, 2=TRACE, 3=SAMPLE, 4=MIRROR, 5=DROP
//	0x06    CircuitState    1B      Exponent: 1=CLOSED, 2=OPEN, 3=HALF_OPEN
//	0x07    Flags           1B      Bitfield (bit 7=CHAOS, 6=CANARY, 5=TRACED, 4=ENCRYPT, 3=SAMPLED, 2=MIRROR, 1=CUSTOM, 0=RSVD)
//	0x08    LatencyHint     2B      Big-endian u16 (upstream latency in microseconds)
//	0x0A    DeployRing      1B      Exponent: 1=PRODUCTION, 2=CANARY, 3=STAGING, 4=DEVELOPMENT
//	0x0B    MeshFlags       1B      Exponent-encoded
//	0x0C    SrcPrefixLo     1B      Raw (low octet of Kingdom ULA source subnet)
//	0x0D    DstPrefixLo     1B      Raw (low octet of Kingdom ULA destination subnet)
//	0x0E    Scratch[0]      1B      Exponent (accumulator high byte for MBC)
//	0x0F    Scratch[1]      1B      Exponent (accumulator low byte for MBC)
//	0x10    Scratch[2]      1B      Exponent (address register high byte for MBC)
//	0x11    Scratch[3]      1B      Exponent (address register low byte for MBC)
//	0x12    Checksum        2B      Big-endian CRC-16/CCITT over octets 0x00-0x11, 18 bytes protected
type MonadRegister struct {
	Version      uint8
	SrcServiceID uint8
	DstServiceID uint8
	HopCount     uint8
	QoSClass     uint8
	FlowAction   uint8
	CircuitState uint8
	Flags        uint8
	LatencyHint  uint16 // Big-endian.
	DeployRing   uint8
	MeshFlags    uint8
	SrcPrefixLo  uint8
	DstPrefixLo  uint8
	Scratch      [4]uint8 // GP registers 0-1 (high/low bytes).
	Checksum     uint16   // CRC-16/CCITT-FALSE over bytes 0x00-0x11.
}

// MonadRegisterSize is the exact wire size. Tests verify this.
const MonadRegisterSize = 20

// Monad flag bits (Flags field, offset 0x07).
const (
	FlagChaos   uint8 = 0x80 // Bit 7: Chaos injection active.
	FlagCanary  uint8 = 0x40 // Bit 6: Canary deployment marker.
	FlagTraced  uint8 = 0x20 // Bit 5: Full trace enabled.
	FlagEncrypt uint8 = 0x10 // Bit 4: Encryption requested.
	FlagSampled uint8 = 0x08 // Bit 3: Sampled for observability.
	FlagMirror  uint8 = 0x04 // Bit 2: Mirrored flow.
	FlagK1      uint8 = 0x02 // Bit 1: Kingdom Mode bit 1.
	FlagK0      uint8 = 0x01 // Bit 0: Kingdom Mode bit 0.
)

// Kingdom Mode values (bits 1:0 of Flags).
// Per IANA: 00=NORMAL, 01=PRIORITY, 10=EXPERIMENTAL, 11=RESERVED.
const (
	KingdomNormal       uint8 = 0x00
	KingdomPriority     uint8 = 0x01
	KingdomExperimental uint8 = 0x02
	KingdomReserved     uint8 = 0x03
)

// Flow action values (FlowAction field, offset 0x05).
const (
	ActionForward uint8 = 0x01
	ActionTrace   uint8 = 0x02
	ActionSample  uint8 = 0x03
	ActionMirror  uint8 = 0x04
	ActionDrop    uint8 = 0x05
)

// Circuit state values (CircuitState field, offset 0x06).
const (
	CircuitClosed   uint8 = 0x01
	CircuitOpen     uint8 = 0x02
	CircuitHalfOpen uint8 = 0x03
)

// === ANAMNESIS EVENT (32 bytes) ===
// Per draft-bellis-unheaded-protocol-foundation-03 §6.1.
// Emitted via BPF ring buffer (ANAMNESIS map, 8 MiB capacity).

// AnamnesisEvent is the 32-byte event emitted by all eBPF programs
// to the ANAMNESIS ring buffer.
// Rust source: ebpf/monad-common/src/lib.rs:657-693
// Per draft-bellis-unheaded-protocol-foundation-01 §6.1
//
// Wire layout (32 bytes):
//
//	Offset  Field           Size    Description
//	0x00    TimestampNs     8B      Kernel monotonic timestamp (bpf_ktime_get_ns, nanoseconds)
//	0x08    EventType       1B      Event type: 0x01=Birth, 0x02=Hop, 0x03=Death, 0x04=Anomaly, 0x05=Chaos
//	0x09    HopID           1B      Sophia-assigned hop identifier (from CONFIG[0] map)
//	0x0A    FlowLabelLo     2B      Low 16 bits of IPv6 Flow Label (big-endian, trace correlation key)
//	0x0C    Monad           20B     Complete 20-byte Monad snapshot at time of event
type AnamnesisEvent struct {
	TimestampNs uint64         // bpf_ktime_get_ns().
	EventType   uint8          // See EventType constants.
	HopID       uint8          // From CONFIG[0] map.
	FlowLabelLo uint16         // Low 16 bits of IPv6 Flow Label for correlation.
	Monad       MonadRegister  // Complete 20-byte Monad snapshot.
}

// AnamnesisEventSize is the exact wire size. Tests verify this.
const AnamnesisEventSize = 32

// Event types for AnamnesisEvent.EventType.
const (
	EventBirth   uint8 = 0x01 // Shield XDP: Monad inserted at ingress.
	EventHop     uint8 = 0x02 // Hop XDP: Per-hop processing completed.
	EventDeath   uint8 = 0x03 // Shield TC: Monad stripped at egress.
	EventAnomaly uint8 = 0x04 // Any: CRC failure, decode error, etc.
	EventChaos   uint8 = 0x05 // Yaldabaoth: Chaos injection applied.
)

// === EXISTING MAP SCHEMAS ===

// BlocklistKey for Shield BLOCKLIST map (u64 → u8).
// Key is low 64 bits of IPv6 source address.
type BlocklistKey struct {
	AddrLo64 uint64 // Low 64 bits of IPv6 address.
}

// BlocklistValue is a simple presence flag.
type BlocklistValue struct {
	Blocked uint8
	_       [7]byte // Pad to 8 bytes for alignment.
}

// RateTokenKey for Shield RATE_TOKENS map (u64 → u32).
type RateTokenKey struct {
	AddrLo64 uint64 // Low 64 bits of IPv6 source address.
}

// RateTokenValue tracks remaining rate limit tokens.
type RateTokenValue struct {
	Remaining uint32 // Tokens remaining in current window.
	_         [4]byte
}

// StatsEntry for all program STATS maps (u32 → u64).
// Index is a program-specific stat ID.
type StatsEntry struct {
	Value uint64 // Saturating counter.
}

// ConfigEntry for all program CONFIG maps (u32 → u64).
// Index is a program-specific config key.
type ConfigEntry struct {
	Value uint64
}

// === SOPHIA MAP (Hop eBPF) ===

// SophiaMapKey encodes (dict_id, value) as a 16-bit key.
// Per Sophia spec: key = (dict_id << 8) | value.
type SophiaMapKey struct {
	Key uint16 // (dict_id << 8) | value.
	_   [6]byte
}

// SophiaMapValue is a 32-byte dictionary entry.
type SophiaMapValue struct {
	Data [32]byte // Sophia entry data (dict-type specific).
}

// CircuitErrorKey for Hop CIRCUIT_ERRORS map.
// Key encodes (src_svc_id | (dst_svc_id << 8)).
type CircuitErrorKey struct {
	Key uint16 // (src_svc_id | (dst_svc_id << 8)).
	_   [6]byte
}

// CircuitErrorValue tracks error count for circuit breaker.
type CircuitErrorValue struct {
	Count uint32
	_     [4]byte
}

// === FLOW TRACKER MAPS ===

// FlowKey is the 5-tuple flow identifier.
// Matches ebpf/common/src/lib.rs FlowKey (IPv4-based, not IPv6).
// Rust source: ebpf/common/src/lib.rs:64-71
//
// Wire layout (16 bytes):
//
//	Offset  Field           Size    Description
//	0x00    SrcAddr         4B      IPv4 source address (network byte order)
//	0x04    DstAddr         4B      IPv4 destination address (network byte order)
//	0x08    SrcPort         2B      Source port (network byte order)
//	0x0A    DstPort         2B      Destination port (network byte order)
//	0x0C    Protocol        1B      IP protocol (6=TCP, 17=UDP)
//	0x0D    _pad            3B      Alignment
type FlowKey struct {
	SrcAddr  uint32  // IPv4 source address (network byte order).
	DstAddr  uint32  // IPv4 destination address (network byte order).
	SrcPort  uint16  // Source port (network byte order).
	DstPort  uint16  // Destination port (network byte order).
	Protocol uint8   // IP protocol (IPPROTO_TCP=6, IPPROTO_UDP=17).
	_        [3]byte // Alignment padding.
}

// FlowKeySize is the exact wire size.
const FlowKeySize = 16 // 4+4+2+2+1+3

// FlowState tracks bidirectional flow state.
// Matches ebpf/common/src/lib.rs FlowState:108-118
// Rust source: ebpf/common/src/lib.rs:108-118
//
// Wire layout (72 bytes):
// Note: 72 bytes, not 56. The original comment had incorrect arithmetic.
// 16 (TraceID) + 6*8 (six uint64) + 1 (State) + 7 (_pad) = 72.
//
//	Offset  Field           Size    Description
//	0x00    TraceID         16B     128-bit trace identifier (high:8B, low:8B)
//	0x10    StartNs         8B      Connection start timestamp (nanoseconds)
//	0x18    LastSeenNs      8B      Last packet timestamp
//	0x20    PacketsIn       8B      Inbound packet count
//	0x28    PacketsOut      8B      Outbound packet count
//	0x30    BytesIn         8B      Inbound bytes
//	0x38    BytesOut        8B      Outbound bytes
//	0x40    State           1B      Connection state enum
//	0x41    _pad            7B      Alignment
type FlowState struct {
	TraceID     [16]byte // 128-bit trace ID (high 8B, low 8B big-endian).
	StartNs     uint64   // Connection start timestamp (bpf_ktime_get_ns).
	LastSeenNs  uint64   // Last packet timestamp.
	PacketsIn   uint64   // Inbound packet count.
	PacketsOut  uint64   // Outbound packet count.
	BytesIn     uint64   // Inbound bytes.
	BytesOut    uint64   // Outbound bytes.
	State       uint8    // Connection state (ConnectionState enum).
	_           [7]byte  // Alignment padding.
}

// FlowStateSize is the exact wire size.
// 16 (TraceID) + 6*8 (six uint64) + 1 (State) + 7 (_pad) = 72.
const FlowStateSize = 72

// TraceAssocKey maps a trace to a flow.
type TraceAssocKey struct {
	FlowHash uint32 // Hash of FlowKey.
	_        [4]byte
}

// TraceAssocValue stores the trace ID.
type TraceAssocValue struct {
	TraceID [16]byte // 128-bit trace identifier.
}

// === CHAOS MAPS (Yaldabaoth) ===

// ChaosTargetKey identifies a chaos target by IPv6 Flow Label.
type ChaosTargetKey struct {
	FlowLabel uint32 // 20-bit Flow Label (zero-extended to u32).
	_         [4]byte
}

// ChaosTargetValue encodes chaos mode and parameters.
// mode = low 32 bits, param = high 32 bits of packed u64.
type ChaosTargetValue struct {
	Mode  uint32 // 1=BitFlip, 2=Delay, 3=Duplicate, 4=Truncate, 5=Marker.
	Param uint32 // Mode-specific parameter.
}

// Chaos modes.
const (
	ChaosBitFlip   uint32 = 0x01
	ChaosDelay     uint32 = 0x02
	ChaosDuplicate uint32 = 0x03
	ChaosTruncate  uint32 = 0x04
	ChaosMarker    uint32 = 0x05
)

// === MONAD CPU MAPS (Doom-over-IPv6) ===

// MbcCpuState is the 104-byte CPU state for the MBC virtual machine.
// Matches monad-cpu-ebpf/src/main.rs MbcCpuState.
// Per Monad spec §12 (computational completeness PoC).
// Rust source: ebpf/monad-cpu-ebpf/src/main.rs (monad_common::MbcCpuState)
//
// Wire layout (104 bytes):
// Note: 104 bytes, not 80. The original comment had incorrect arithmetic.
// 64 ([16]uint32) + 4 (PC) + 4 (Flags+Halted+Stalled+_pad) + 4*8 (four uint64) = 104.
//
//	Offset  Field           Size    Description
//	0x00    Registers       64B     [16]u32 (r0-r15)
//	0x40    PC              4B      Program counter
//	0x44    Flags           1B      CPU flags (Z, N, C bits)
//	0x45    Halted          1B      1 if HALT executed
//	0x46    Stalled         1B      1 if waiting for cache miss
//	0x47    _pad            1B      Alignment
//	0x48    SleepUntilNs    8B      bpf_ktime_get_ns target
//	0x50    InsnCount       8B      Total instructions executed
//	0x58    CacheHits       8B      L1 cache hit count
//	0x60    CacheMisses     8B      L1 cache miss count
type MbcCpuState struct {
	Registers     [16]uint32 // r0-r15 general purpose registers (64 bytes).
	PC            uint32     // Program counter.
	Flags         uint8      // CPU flags: bit 0=Z, bit 1=N, bit 2=C.
	Halted        uint8      // 1 if HALT instruction executed.
	Stalled       uint8      // 1 if waiting for cache miss (D-003).
	_pad          uint8      // Alignment padding.
	SleepUntilNs  uint64     // bpf_ktime_get_ns() sleep target.
	InsnCount     uint64     // Total instructions executed (for IPS stats).
	CacheHits     uint64     // L1 cache hits.
	CacheMisses   uint64     // L1 cache misses.
}

// MbcCpuStateSize is the exact wire size. Tests verify this.
// 64 ([16]uint32) + 4 (PC) + 4 (Flags+Halted+Stalled+_pad) + 4*8 (four uint64) = 104.
const MbcCpuStateSize = 104

// CacheLineKey for Monad CPU L1_CACHE map.
type CacheLineKey struct {
	Addr uint32 // Cache line address (word-aligned).
	_    [4]byte
}

// CacheLineValue is a 64-byte cache line.
type CacheLineValue struct {
	Data [64]byte
}

// === FLOW TRACKER EXTENSION MAPS (IPv4-based) ===

// FlowMigrationTokenValue holds migration token state for Flow Tracker.
// Key: FlowKey (same 5-tuple as FLOWS map, 16 bytes).
// Value: migration token state (48 bytes).
// Map type: HashMap (LRU behavior via userspace TTL).
// Per RFC 9000 §9 — Flow migration validation tokens.
//
// Wire layout (48 bytes):
//
//	Offset  Field           Size    Description
//	0x00    Token           16B     128-bit migration token
//	0x10    ExpiryNs        8B      Expiry timestamp (bpf_ktime_get_ns)
//	0x18    NewSrcAddr      4B      New source address after migration
//	0x1C    NewDstAddr      4B      New destination address after migration
//	0x20    NewSrcPort      2B      New source port
//	0x22    NewDstPort      2B      New destination port
//	0x24    Flags           4B      Migration flags (bit 0: active)
//	0x28    _pad            4B      Alignment padding
type FlowMigrationTokenValue struct {
	Token      [16]byte // 128-bit migration token
	ExpiryNs   uint64   // Expiry timestamp (bpf_ktime_get_ns)
	NewSrcAddr uint32   // New source address after migration
	NewDstAddr uint32   // New destination address after migration
	NewSrcPort uint16   // New source port
	NewDstPort uint16   // New destination port
	Flags      uint32   // Migration flags (bit 0: active)
	_          [4]byte  // Alignment padding to 48 bytes
}

// FlowMigrationTokenValueSize is the exact wire size.
const FlowMigrationTokenValueSize = 48 // 16+8+4+4+2+2+4+4

// FlowCancelValue holds cancellation state for Flow Tracker.
// Key: FlowKey (same 5-tuple as FLOWS map, 16 bytes).
// Value: cancellation state (24 bytes).
// Map type: HashMap (LRU behavior via userspace TTL).
// Per RFC 9114 §4.1.1 — Per-flow cancellation state.
//
// Wire layout (24 bytes):
// Go alignment: uint64 field requires 8-byte alignment, so 4 bytes of
// implicit padding are inserted after Reason (uint32). Rust #[repr(C, packed)]
// has no such padding, placing TimestampNs at offset 0x04. The Go offsets are:
//
//	Offset  Field           Size    Description
//	0x00    Reason          4B      Cancellation reason code
//	0x04    (padding)       4B      Go alignment for uint64
//	0x08    TimestampNs     8B      When cancellation was requested
//	0x10    Flags           4B      Bit 0: active, Bit 1: send-rst
//	0x14    _pad            4B      Explicit padding
//
// Total: 4 + 4(implicit) + 8 + 4 + 4 = 24 bytes.
type FlowCancelValue struct {
	Reason      uint32 // Cancellation reason code
	TimestampNs uint64 // When cancellation was requested
	Flags       uint32 // Bit 0: active, Bit 1: send-rst
	_           [4]byte // Alignment padding to 24 bytes
}

// FlowCancelValueSize is the exact wire size.
// 4 (Reason) + 4 (Go alignment padding) + 8 (TimestampNs) + 4 (Flags) + 4 (_pad) = 24.
const FlowCancelValueSize = 24

// === MAP CONFIGURATION CONSTANTS ===

// Standard map sizes matching eBPF program definitions.
const (
	AnamnesisRingSize     = 8 * 1024 * 1024  // 8 MiB per ring buffer.
	FlowEventsRingSize    = 256 * 1024        // 256 KiB.
	LatencyEventsRingSize = 256 * 1024        // 256 KiB.
	PacketEventsRingSize  = 256 * 1024        // 256 KiB.
	SyscallEventsRingSize = 256 * 1024        // 256 KiB.
	ComputeEventsRingSize = 256 * 1024        // 256 KiB.

	BlocklistMaxEntries           = 4096
	RateTokensMaxEntries          = 4096
	SophiaMaxEntries              = 65536
	CircuitErrorsMaxEntries       = 65536
	FlowsMaxEntries               = 16384
	FlowMigrationTokensMaxEntries = 4096
	FlowCancelFlowsMaxEntries      = 4096
	ChaosTargetsMaxEntries        = 4096
	ROMMapMaxEntries              = 262144   // 1 MiB of instructions.
	RAMMapMaxEntries              = 2097152  // 8 MiB word-addressed.
	ScreenMapMaxEntries           = 64000    // 320×200 framebuffer.
	CPUMapMaxEntries              = 256      // Per-flow CPU instances.
	L1CacheMaxEntries             = 256      // 256 cache lines × 64 bytes.
	StatsMaxEntries               = 32
	ConfigMaxEntries              = 16
)
