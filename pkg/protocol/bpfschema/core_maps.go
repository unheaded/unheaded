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
//
// Wire layout:
//
//	Offset  Field           Size    Encoding
//	0x00    Version         1B      Raw u8
//	0x01    SrcServiceID    1B      Exponent (Sophia dict 0x03)
//	0x02    DstServiceID    1B      Exponent (Sophia dict 0x03)
//	0x03    HopCount        1B      Raw u8 (decremented per hop)
//	0x04    QoSClass        1B      Exponent (Sophia dict 0x01)
//	0x05    FlowAction      1B      Exponent {FORWARD=1,TRACE=2,SAMPLE=3,MIRROR=4,DROP=5}
//	0x06    CircuitState    1B      Exponent {CLOSED=1,OPEN=2,HALF_OPEN=3}
//	0x07    Flags           1B      Bitfield: CHAOS|CANARY|TRACED|ENCRYPT|SAMPLED|MIRROR|K1|K0
//	0x08    LatencyHint     2B      Raw u16 BE (microseconds)
//	0x0A    DeployRing      1B      Exponent {PROD=1,CANARY=2,STAGING=3,DEV=4}
//	0x0B    MeshFlags       1B      Exponent
//	0x0C    SrcPrefixLo     1B      Raw (Kingdom ULA source subnet low byte)
//	0x0D    DstPrefixLo     1B      Raw (Kingdom ULA dest subnet low byte)
//	0x0E    Scratch0        1B      Exponent (GP register 0 high)
//	0x0F    Scratch1        1B      Exponent (GP register 0 low)
//	0x10    Scratch2        1B      Exponent (GP register 1 high)
//	0x11    Scratch3        1B      Exponent (GP register 1 low)
//	0x12    Checksum        2B      CRC-16/CCITT over 0x00-0x11
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
//
// Wire layout:
//
//	Offset  Field           Size    Description
//	0x00    TimestampNs     8B      Kernel monotonic nanoseconds (bpf_ktime_get_ns)
//	0x08    EventType       1B      Event type enum
//	0x09    HopID           1B      Hop identifier (from CONFIG map)
//	0x0A    FlowLabelLo     2B      Low 16 bits of IPv6 Flow Label
//	0x0C    Monad           20B     Complete Monad snapshot at event time
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
// Matches flow-tracker/src/main.rs FlowKey.
type FlowKey struct {
	SrcAddr  [16]byte // IPv6 source.
	DstAddr  [16]byte // IPv6 destination.
	SrcPort  uint16
	DstPort  uint16
	Protocol uint8
	_        [3]byte
}

// FlowKeySize is the exact wire size.
const FlowKeySize = 40 // 16+16+2+2+1+3

// FlowState tracks bidirectional flow state.
type FlowState struct {
	PacketsIn  uint64
	PacketsOut uint64
	BytesIn    uint64
	BytesOut   uint64
	TCPState   uint8  // TCP state machine position.
	Direction  uint8  // 0=unknown, 1=client→server, 2=server→client.
	_          [6]byte
}

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

// MbcCpuState is the 80-byte CPU state for the MBC virtual machine.
// Matches monad-cpu-ebpf/src/main.rs MbcCpuState.
// Per Monad spec §12 (computational completeness PoC).
type MbcCpuState struct {
	Registers  [16]uint32 // r0-r15 general purpose registers.
	PC         uint32     // Program counter.
	Flags      uint32     // CPU flags (zero, carry, overflow, etc).
	SleepUntil uint64     // bpf_ktime_get_ns target for sleep.
}

// MbcCpuStateSize is the exact wire size. Tests verify this.
const MbcCpuStateSize = 80

// CacheLineKey for Monad CPU L1_CACHE map.
type CacheLineKey struct {
	Addr uint32 // Cache line address (word-aligned).
	_    [4]byte
}

// CacheLineValue is a 64-byte cache line.
type CacheLineValue struct {
	Data [64]byte
}

// === MAP CONFIGURATION CONSTANTS ===

// Standard map sizes matching eBPF program definitions.
const (
	AnamnesisRingSize     = 8 * 1024 * 1024  // 8 MiB per ring buffer.
	FlowEventsRingSize    = 256 * 1024        // 256 KiB.
	LatencyEventsRingSize = 256 * 1024        // 256 KiB.
	PacketEventsRingSize  = 256 * 1024        // 256 KiB.
	SyscallEventsRingSize = 256 * 1024        // 256 KiB.
	ComputeEventsRingSize = 256 * 1024        // 256 KiB.

	BlocklistMaxEntries      = 4096
	RateTokensMaxEntries     = 4096
	SophiaMaxEntries         = 65536
	CircuitErrorsMaxEntries  = 65536
	FlowsMaxEntries          = 16384
	ChaosTargetsMaxEntries   = 4096
	ROMMapMaxEntries         = 262144   // 1 MiB of instructions.
	RAMMapMaxEntries         = 2097152  // 8 MiB word-addressed.
	ScreenMapMaxEntries      = 64000    // 320×200 framebuffer.
	CPUMapMaxEntries         = 256      // Per-flow CPU instances.
	L1CacheMaxEntries        = 256      // 256 cache lines × 64 bytes.
	StatsMaxEntries          = 32
	ConfigMaxEntries         = 16
)
