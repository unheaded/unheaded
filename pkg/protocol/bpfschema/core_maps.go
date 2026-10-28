// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// core_maps.go defines BPF map key/value structs for the EXISTING eBPF programs
// (Shield, Hop, Yaldabaoth, Flow Tracker, Monad CPU, Latency Probe,
// Packet Marker, Syscall Tracer).
//
// These structs follow the Rust definitions (rust_source_test.go enforces
// it) in:
//   - ebpf/monad-common/src/lib.rs (Monad, AnamnesisEvent, CrcCcitt)
//   - ebpf/shield-ebpf/src/main.rs (BLOCKLIST, RATE_TOKENS, STATS)
//   - ebpf/hop-ebpf/src/main.rs (SOPHIA, CIRCUIT_ERRORS, CONFIG)
//   - ebpf/flow-tracker/src/main.rs (FlowKey, FlowState, TraceId)
//   - ebpf/monad-cpu-ebpf/src/main.rs (MbcCpuState, ComputeHopEvent)
//
// Wire format per draft-bellis-unheaded-protocol-foundation-03 §3.
// All multi-byte fields big-endian per RFC 8200.

package bpfschema

import (
	"encoding/binary"
	"strconv"
)

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
//	0x05    FlowAction      1B      Exponent: 0=FORWARD, 1=TRACE, 2=SAMPLE, 3=DROP, 4=MIRROR (foundation-06)
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

// Monad flag bits (Flags field, offset 0x07). Names and values follow the
// foundation-06 Flags Bitfield registry (C Y T E S M CUSTOM RSVD), the same
// names as pkg/monad and ebpf/monad-common flags. Bits 1:0 are two
// independent flags, not a 2-bit field: the Kingdom Mode registry in
// foundation-06 is a separate concept and is not carried here.
const (
	FlagChaos    uint8 = 0x80 // Bit 7: CHAOS, chaos injection active.
	FlagCanary   uint8 = 0x40 // Bit 6: CANARY, canary deployment path.
	FlagTraced   uint8 = 0x20 // Bit 5: TRACED, full trace enabled.
	FlagEncrypt  uint8 = 0x10 // Bit 4: ENCRYPT, payload encrypted.
	FlagSampled  uint8 = 0x08 // Bit 3: SAMPLED, statistically sampled.
	FlagMirror   uint8 = 0x04 // Bit 2: MIRROR, mirror copy.
	FlagCustom   uint8 = 0x02 // Bit 1: CUSTOM, scratch and checksum carry exponent-encoded values.
	FlagReserved uint8 = 0x01 // Bit 0: RSVD, senders MUST set to zero.
)

// Deprecated aliases for the old "Kingdom Mode bit" names; removed after
// one release (S-TRUE-WIRE Step 102).
const (
	// Deprecated: use FlagCustom.
	FlagK1 = FlagCustom
	// Deprecated: use FlagReserved.
	FlagK0 = FlagReserved
)

// Flow action values (FlowAction field, offset 0x05): the foundation-06
// "Monad Flow Actions" registry. Before S-TRUE-WIRE (Step 84, 2026-10-27) the
// code used FORWARD 0x01, TRACE 0x02, SAMPLE 0x03, MIRROR 0x04, DROP 0x05;
// the spec wins (the E4 rule) and FlowActionLegacy decodes the old numbering
// for one release. An all-zero Monad now reads as FORWARD, the spec default.
const (
	ActionForward       uint8 = 0x00 // FORWARD: normal forwarding (default).
	ActionTrace         uint8 = 0x01 // TRACE: full event logging to Anamnesis.
	ActionSample        uint8 = 0x02 // SAMPLE: probabilistic event logging.
	ActionDrop          uint8 = 0x03 // DROP: discard packet.
	ActionMirror        uint8 = 0x04 // MIRROR: clone to monitoring interface.
	ActionRateLimit     uint8 = 0x05 // RATE_LIMIT: apply rate limiting policy.
	ActionRedirect      uint8 = 0x06 // REDIRECT: redirect to alternate destination.
	ActionInject        uint8 = 0x07 // INJECT: inject synthetic packet.
	ActionKeyRotate     uint8 = 0x10 // KEY_ROTATE: PQC key rotation event.
	ActionKeyRevoke     uint8 = 0x11 // KEY_REVOKE: PQC key revocation event.
	ActionKeyDistribute uint8 = 0x12 // KEY_DISTRIBUTE: PQC key distribution event.
	ActionKeyVerify     uint8 = 0x13 // KEY_VERIFY: PQC key verification event.
	ActionKeyChallenge  uint8 = 0x14 // KEY_CHALLENGE: PQC challenge-response.
)

// FlowActionLegacy maps a flow-action byte written by a pre-S-TRUE-WIRE
// producer (FORWARD 0x01 .. DROP 0x05) to its foundation-06 value. ok is false
// for bytes the old numbering never defined. The two numberings overlap
// (legacy SAMPLE 0x03 is spec DROP), so the caller must know the producer is
// legacy; the byte alone cannot say. Removed one release after S-TRUE-WIRE.
func FlowActionLegacy(v uint8) (uint8, bool) {
	switch v {
	case 0x01:
		return ActionForward, true
	case 0x02:
		return ActionTrace, true
	case 0x03:
		return ActionSample, true
	case 0x04:
		return ActionMirror, true
	case 0x05:
		return ActionDrop, true
	}
	return 0, false
}

// FlowActionName returns the lower-case foundation-06 registry name of a
// flow-action value, or "unknown(N)".
func FlowActionName(v uint8) string {
	switch v {
	case ActionForward:
		return "forward"
	case ActionTrace:
		return "trace"
	case ActionSample:
		return "sample"
	case ActionDrop:
		return "drop"
	case ActionMirror:
		return "mirror"
	case ActionRateLimit:
		return "rate_limit"
	case ActionRedirect:
		return "redirect"
	case ActionInject:
		return "inject"
	case ActionKeyRotate:
		return "key_rotate"
	case ActionKeyRevoke:
		return "key_revoke"
	case ActionKeyDistribute:
		return "key_distribute"
	case ActionKeyVerify:
		return "key_verify"
	case ActionKeyChallenge:
		return "key_challenge"
	}
	return "unknown(" + strconv.Itoa(int(v)) + ")"
}

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
//	0x08    EventType       1B      Event type: foundation-06 registry, 0x00=BORN .. 0x08=ANOMALY
//	0x09    HopID           1B      Sophia-assigned hop identifier (from CONFIG[0] map)
//	0x0A    FlowLabelLo     2B      Low 16 bits of IPv6 Flow Label (big-endian, trace correlation key)
//	0x0C    Monad           20B     Complete 20-byte Monad snapshot at time of event
type AnamnesisEvent struct {
	TimestampNs uint64        // bpf_ktime_get_ns().
	EventType   uint8         // See EventType constants.
	HopID       uint8         // From CONFIG[0] map.
	FlowLabelLo uint16        // Low 16 bits of IPv6 Flow Label for correlation.
	Monad       MonadRegister // Complete 20-byte Monad snapshot.
}

// AnamnesisEventSize is the exact wire size. Tests verify this.
const AnamnesisEventSize = 32

// Event types for AnamnesisEvent.EventType: the foundation-06 "Anamnesis
// Event Types" registry. Before S-TRUE-WIRE (Step 91, 2026-10-27) the
// programs emitted Birth 0x01, Hop 0x02, Death 0x03 on the spec's COMPUTED,
// WOTAN_RD and WOTAN_WR codes; E4 (2026-10-26) had already moved CHAOS and
// ANOMALY. EventTypeLegacy decodes the old numbering for one release.
const (
	EventBorn     uint8 = 0x00 // EVENT_BORN: Shield XDP inserted the Monad at ingress.
	EventComputed uint8 = 0x01 // EVENT_COMPUTED: a hop executed its Shim step, Monad updated.
	EventWotanRd  uint8 = 0x02 // EVENT_WOTAN_RD: Wotan memory read (no producer yet).
	EventWotanWr  uint8 = 0x03 // EVENT_WOTAN_WR: Wotan memory write (no producer yet).
	EventChaos    uint8 = 0x04 // EVENT_CHAOS: Yaldabaoth chaos injection applied.
	EventRollback uint8 = 0x05 // EVENT_ROLLBACK: Monad rolled back (no producer yet).
	EventDied     uint8 = 0x06 // EVENT_DIED: Shield TC stripped the Monad at egress.
	EventKeyOp    uint8 = 0x07 // EVENT_KEY_OP: PQC key lifecycle event (no producer yet).
	EventAnomaly  uint8 = 0x08 // EVENT_ANOMALY: CRC failure, decode error, version error.
)

// Deprecated event-type names, kept for one release after S-TRUE-WIRE. They
// carry the spec values of the events they always meant.
const (
	// Deprecated: use EventBorn.
	EventBirth = EventBorn
	// Deprecated: use EventComputed (a hop is a computed step).
	EventHop = EventComputed
	// Deprecated: use EventDied.
	EventDeath = EventDied
)

// EventTypeLegacy maps an event-type byte from a pre-S-TRUE-WIRE object
// (Birth 0x01, Hop 0x02, Death 0x03, Chaos 0x04, Anomaly 0x08) to its
// foundation-06 value. ok is false for bytes the old objects never emitted.
// The numberings overlap (legacy Birth 0x01 is spec COMPUTED), so the reader
// must know which object produced the ring; the byte alone cannot say.
func EventTypeLegacy(v uint8) (uint8, bool) {
	switch v {
	case 0x01:
		return EventBorn, true
	case 0x02:
		return EventComputed, true
	case 0x03:
		return EventDied, true
	case 0x04:
		return EventChaos, true
	case 0x08:
		return EventAnomaly, true
	}
	return 0, false
}

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
	TraceID    [16]byte // 128-bit trace ID (high 8B, low 8B big-endian).
	StartNs    uint64   // Connection start timestamp (bpf_ktime_get_ns).
	LastSeenNs uint64   // Last packet timestamp.
	PacketsIn  uint64   // Inbound packet count.
	PacketsOut uint64   // Outbound packet count.
	BytesIn    uint64   // Inbound bytes.
	BytesOut   uint64   // Outbound bytes.
	State      uint8    // Connection state (ConnectionState enum).
	_          [7]byte  // Alignment padding.
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

// MbcCpuState is the 136-byte CPU state for the MBC virtual machine
// (default features). Built with --features ascend-linux the Rust struct
// appends rv2mbc_base and 4 pad bytes (MbcCpuStateAscendSize, 144); this Go
// mirror carries the default layout.
// Per Monad spec §12 (computational completeness PoC).
// Rust source: ebpf/monad-common/src/lib.rs MbcCpuState (ADR-067, ADR-077).
//
// Wire layout (136 bytes):
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
//	0x68    InterruptPending 1B     Interrupt waiting flag
//	0x69    InterruptVector  1B     Pending interrupt vector
//	0x6A    InterruptsEnabled 1B    Interrupt enable flag
//	0x6B    _pad2           1B      Alignment
//	0x6C    TickCounter     4B      Timer tick counter
//	0x70    ProgramBreak    4B      Heap end (SYS_BRK)
//	0x74    ExitCode        4B      Exit code (SYS_EXIT)
//	0x78    CurrentPID      1B      Current process ID (Level 4c)
//	0x79    NumProcesses    1B      Active process count (Level 4c)
//	0x7A    MmuEnabled      1B      MMU enabled flag (Level 4d)
//	0x7B    _pad3           1B      Alignment padding
//	0x7C    PageDirBase     4B      Page directory base address (Level 4d)
//	0x80    PrivLevel       1B      RV32 privilege: 0=M, 1=S, 3=U (ADR-067)
//	0x81    _pad4.._pad6    3B      Alignment
//	0x84    ReservationAddress 4B   LR.W reservation, 0xFFFFFFFF = none (ADR-067)
type MbcCpuState struct {
	Registers          [16]uint32 // r0-r15 general purpose registers (64 bytes).
	PC                 uint32     // Program counter.
	Flags              uint8      // CPU flags: bit 0=Z, bit 1=N, bit 2=C.
	Halted             uint8      // 1 if HALT instruction executed.
	Stalled            uint8      // 1 if waiting for cache miss (D-003).
	_                  uint8      // Alignment padding.
	SleepUntilNs       uint64     // bpf_ktime_get_ns() sleep target.
	InsnCount          uint64     // Total instructions executed (for IPS stats).
	CacheHits          uint64     // L1 cache hits.
	CacheMisses        uint64     // L1 cache misses.
	InterruptPending   uint8      // Non-zero when interrupt waiting.
	InterruptVector    uint8      // Vector number of pending interrupt.
	InterruptsEnabled  uint8      // Non-zero when CPU accepts interrupts.
	_                  uint8      // Alignment padding.
	TickCounter        uint32     // Tick counter for timer interrupt generation.
	ProgramBreak       uint32     // Heap end address for SYS_BRK (Level 4b).
	ExitCode           uint32     // Exit code from SYS_EXIT (Level 4b).
	CurrentPID         uint8      // Current process ID (0-3). Level 4c scheduler.
	NumProcesses       uint8      // Number of active processes. Level 4c scheduler.
	MmuEnabled         uint8      // MMU enabled (0=flat, 1=paging). Level 4d.
	_                  uint8      // Alignment padding.
	PageDirBase        uint32     // Page directory base address. Level 4d.
	PrivLevel          uint8      // Privilege level: 0=M, 1=S, 3=U (ADR-067).
	_                  uint8      // Alignment padding.
	_                  uint8      // Alignment padding.
	_                  uint8      // Alignment padding.
	ReservationAddress uint32     // LR.W reservation address; 0xFFFFFFFF = none (ADR-067).
}

// MbcCpuStateSize is the exact wire size (default features); rust_source_test.go
// derives it from the Rust struct. 128 bytes up to PageDirBase, then PrivLevel,
// three pad bytes and ReservationAddress = 136.
const MbcCpuStateSize = 136

// MbcCpuStateAscendSize is the size with --features ascend-linux (rv2mbc_base + pad).
const MbcCpuStateAscendSize = 144

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
// Value: migration token state (44 bytes).
// Map type: HashMap (LRU behavior via userspace TTL).
// Per RFC 9000 §9 — Flow migration validation tokens.
//
// The Rust struct is #[repr(C, packed)], so it is 44 bytes, not the 48 its
// comment claims. ExpiryNs is a byte array so the Go struct has 4-byte
// alignment and the same 44-byte size; use Expiry/SetExpiry.
//
// Wire layout (44 bytes):
//
//	Offset  Field           Size    Description
//	0x00    Token           16B     128-bit migration token
//	0x10    ExpiryNs        8B      Expiry timestamp (bpf_ktime_get_ns)
//	0x18    NewSrcAddr      4B      New source address after migration
//	0x1C    NewDstAddr      4B      New destination address after migration
//	0x20    NewSrcPort      2B      New source port
//	0x22    NewDstPort      2B      New destination port
//	0x24    Flags           4B      Migration flags (bit 0: active)
//	0x28    _pad            4B      Padding
type FlowMigrationTokenValue struct {
	Token      [16]byte // 128-bit migration token
	ExpiryNs   [8]byte  // Expiry timestamp (bpf_ktime_get_ns), host byte order; see Expiry.
	NewSrcAddr uint32   // New source address after migration
	NewDstAddr uint32   // New destination address after migration
	NewSrcPort uint16   // New source port
	NewDstPort uint16   // New destination port
	Flags      uint32   // Migration flags (bit 0: active)
	_          [4]byte  // Padding (Rust _pad).
}

// Expiry returns ExpiryNs as written by the eBPF program (host byte order).
func (v *FlowMigrationTokenValue) Expiry() uint64 {
	return binary.NativeEndian.Uint64(v.ExpiryNs[:])
}

// SetExpiry stores ns into ExpiryNs in host byte order.
func (v *FlowMigrationTokenValue) SetExpiry(ns uint64) {
	binary.NativeEndian.PutUint64(v.ExpiryNs[:], ns)
}

// FlowMigrationTokenValueSize is the exact wire size (packed).
const FlowMigrationTokenValueSize = 44 // 16+8+4+4+2+2+4+4

// FlowCancelValue holds cancellation state for Flow Tracker.
// Key: FlowKey (same 5-tuple as FLOWS map, 16 bytes).
// Value: cancellation state (20 bytes).
// Map type: HashMap (LRU behavior via userspace TTL).
// Per RFC 9114 §4.1.1 — Per-flow cancellation state.
//
// The Rust struct is #[repr(C, packed)]: TimestampNs sits at 0x04 with no
// padding and the value is 20 bytes, not the 24 its comment claims.
// TimestampNs is a byte array so the Go struct matches; use Timestamp.
//
// Wire layout (20 bytes):
//
//	Offset  Field           Size    Description
//	0x00    Reason          4B      Cancellation reason code
//	0x04    TimestampNs     8B      When cancellation was requested (host byte order)
//	0x0C    Flags           4B      Bit 0: active, Bit 1: send-rst
//	0x10    _pad            4B      Padding
type FlowCancelValue struct {
	Reason      uint32  // Cancellation reason code
	TimestampNs [8]byte // When cancellation was requested; see Timestamp.
	Flags       uint32  // Bit 0: active, Bit 1: send-rst
	_           [4]byte // Padding (Rust _pad).
}

// Timestamp returns TimestampNs as written by the eBPF program (host byte order).
func (v *FlowCancelValue) Timestamp() uint64 {
	return binary.NativeEndian.Uint64(v.TimestampNs[:])
}

// SetTimestamp stores ns into TimestampNs in host byte order.
func (v *FlowCancelValue) SetTimestamp(ns uint64) {
	binary.NativeEndian.PutUint64(v.TimestampNs[:], ns)
}

// FlowCancelValueSize is the exact wire size (packed): 4 + 8 + 4 + 4 = 20.
const FlowCancelValueSize = 20

// === MAP CONFIGURATION CONSTANTS ===

// Standard map sizes matching eBPF program definitions.
const (
	AnamnesisRingSize     = 8 * 1024 * 1024 // 8 MiB per ring buffer.
	FlowEventsRingSize    = 256 * 1024      // 256 KiB.
	LatencyEventsRingSize = 256 * 1024      // 256 KiB.
	PacketEventsRingSize  = 256 * 1024      // 256 KiB.
	SyscallEventsRingSize = 256 * 1024      // 256 KiB.
	ComputeEventsRingSize = 256 * 1024      // 256 KiB.

	BlocklistMaxEntries           = 4096
	RateTokensMaxEntries          = 4096
	SophiaMaxEntries              = 65536
	CircuitErrorsMaxEntries       = 65536
	FlowsMaxEntries               = 16384
	FlowMigrationTokensMaxEntries = 4096
	FlowCancelFlowsMaxEntries     = 4096
	ChaosTargetsMaxEntries        = 4096
	ROMMapMaxEntries              = 262144  // 1 MiB of instructions.
	RAMMapMaxEntries              = 2097152 // 8 MiB word-addressed.
	ScreenMapMaxEntries           = 16000   // 160×100 framebuffer.
	CPUMapMaxEntries              = 256     // Per-flow CPU instances.
	L1CacheMaxEntries             = 256     // 256 cache lines × 64 bytes.
	StatsMaxEntries               = 32
	ConfigMaxEntries              = 16
)
