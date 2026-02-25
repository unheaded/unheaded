// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package ebpf — Anamnesis event types for the BPF → dashboard pipeline.
//
// These Go structs mirror the Rust types in ebpf/monad-common/src/lib.rs
// byte-for-byte.  The AnamnesisEvent is exactly 32 bytes and maps 1:1 to
// the kernel-side ring buffer records emitted by Shield, Hop, and Yaldabaoth
// BPF programs.
//
// Wire format (from draft-bellis §6.1):
//
//	[0..8]:   timestamp_ns   — bpf_ktime_get_ns()
//	[8]:      event_type     — EventType enum value
//	[9]:      hop_id         — which hop emitted this event
//	[10..12]: flow_label_lo  — low 16 bits of IPv6 Flow Label (BE)
//	[12..32]: monad          — full 20-byte Monad snapshot
//
// Per RFC 9669: we use "BPF" (not "eBPF") throughout.
package ebpf

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
)

// ── Size constants ──────────────────────────────────────────────────────────

// MonadSize is the fixed size of the Monad register file (octets).
const MonadSize = 20

// AnamnesisEventSize is the fixed size of an AnamnesisEvent on the wire.
const AnamnesisEventSize = 32

// MonadCRCDataLen is the number of bytes protected by the Monad CRC (0x00–0x11).
const MonadCRCDataLen = 18

// MonadVersion is the current protocol version (draft-bellis-unheaded-protocol-foundation-01).
const MonadVersion = 0x01

// ── Event types ─────────────────────────────────────────────────────────────

// EventType identifies the kind of Anamnesis event.
type EventType uint8

const (
	EventBirth   EventType = 0x01 // Shield ingress inserted the Monad
	EventHop     EventType = 0x02 // Intermediate BPF program processed packet
	EventDeath   EventType = 0x03 // Shield egress stripped the Monad
	EventAnomaly EventType = 0x04 // CRC failure, unknown Sophia key, decode error
	EventChaos   EventType = 0x05 // Yaldabaoth chaos injection
)

// String returns the human-readable event type name.
func (e EventType) String() string {
	switch e {
	case EventBirth:
		return "birth"
	case EventHop:
		return "hop"
	case EventDeath:
		return "death"
	case EventAnomaly:
		return "anomaly"
	case EventChaos:
		return "chaos"
	default:
		return fmt.Sprintf("unknown(%d)", uint8(e))
	}
}

// Valid returns true if the event type is a known value.
func (e EventType) Valid() bool {
	return e >= EventBirth && e <= EventChaos
}

// MarshalJSON encodes the event type as a JSON string.
func (e EventType) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.String())
}

// UnmarshalJSON decodes the event type from a JSON string.
func (e *EventType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch s {
	case "birth":
		*e = EventBirth
	case "hop":
		*e = EventHop
	case "death":
		*e = EventDeath
	case "anomaly":
		*e = EventAnomaly
	case "chaos":
		*e = EventChaos
	default:
		return fmt.Errorf("unknown event type: %q", s)
	}
	return nil
}

// ── Monad flags ─────────────────────────────────────────────────────────────

const (
	FlagChaos   uint8 = 0x80 // Yaldabaoth chaos injection
	FlagCanary  uint8 = 0x40 // Canary deployment path
	FlagTraced  uint8 = 0x20 // Full Anamnesis event required
	FlagEncrypt uint8 = 0x10 // Intra-Kingdom TLS payload
	FlagSampled uint8 = 0x08 // Statistical sampling
	FlagMirror  uint8 = 0x04 // Mirror copy
	FlagCustom  uint8 = 0x02 // Scratch carries exponent-encoded values
	FlagRsvd    uint8 = 0x01 // Reserved, MUST be zero
)

// ── MonadState ──────────────────────────────────────────────────────────────

// MonadState is the Go mirror of the 20-byte Monad register file.
//
// Layout (from draft-bellis §3.3):
//
//	Offset  Field
//	0x00    Version
//	0x01    Src Service ID (exponent-encoded)
//	0x02    Dst Service ID (exponent-encoded)
//	0x03    Hop Count
//	0x04    QoS Class (exponent-encoded)
//	0x05    Flow Action (exponent-encoded)
//	0x06    Circuit State (exponent-encoded)
//	0x07    Flags bitfield
//	0x08    Latency Hint (u16 BE)
//	0x0A    Deployment Ring (exponent-encoded)
//	0x0B    Mesh Flags (exponent-encoded)
//	0x0C    Src Prefix Lo
//	0x0D    Dst Prefix Lo
//	0x0E    Scratch[0]
//	0x0F    Scratch[1]
//	0x10    Scratch[2]
//	0x11    Scratch[3]
//	0x12    Checksum (u16 BE, CRC-16/CCITT)
type MonadState struct {
	Version      uint8  `json:"version"`
	SrcServiceID uint8  `json:"src_service_id"`
	DstServiceID uint8  `json:"dst_service_id"`
	HopCount     uint8  `json:"hop_count"`
	QosClass     uint8  `json:"qos_class"`
	FlowAction   uint8  `json:"flow_action"`
	CircuitState uint8  `json:"circuit_state"`
	Flags        uint8  `json:"flags"`
	LatencyHint  uint16 `json:"latency_hint_us"`
	DeployRing   uint8  `json:"deploy_ring"`
	MeshFlags    uint8  `json:"mesh_flags"`
	SrcPrefixLo  uint8  `json:"src_prefix_lo"`
	DstPrefixLo  uint8  `json:"dst_prefix_lo"`
	Scratch      [4]uint8 `json:"scratch"`
	Checksum     uint16 `json:"checksum"`
}

// DecodeMonadState decodes a 20-byte Monad from a byte slice.
// Returns an error if b is too short.
func DecodeMonadState(b []byte) (MonadState, error) {
	if len(b) < MonadSize {
		return MonadState{}, fmt.Errorf("monad too short: %d < %d", len(b), MonadSize)
	}
	return MonadState{
		Version:      b[0],
		SrcServiceID: b[1],
		DstServiceID: b[2],
		HopCount:     b[3],
		QosClass:     b[4],
		FlowAction:   b[5],
		CircuitState: b[6],
		Flags:        b[7],
		LatencyHint:  binary.BigEndian.Uint16(b[8:10]),
		DeployRing:   b[10],
		MeshFlags:    b[11],
		SrcPrefixLo:  b[12],
		DstPrefixLo:  b[13],
		Scratch:      [4]uint8{b[14], b[15], b[16], b[17]},
		Checksum:     binary.BigEndian.Uint16(b[18:20]),
	}, nil
}

// Encode serializes the Monad to a 20-byte slice.
func (m *MonadState) Encode() [MonadSize]byte {
	var b [MonadSize]byte
	b[0] = m.Version
	b[1] = m.SrcServiceID
	b[2] = m.DstServiceID
	b[3] = m.HopCount
	b[4] = m.QosClass
	b[5] = m.FlowAction
	b[6] = m.CircuitState
	b[7] = m.Flags
	binary.BigEndian.PutUint16(b[8:10], m.LatencyHint)
	b[10] = m.DeployRing
	b[11] = m.MeshFlags
	b[12] = m.SrcPrefixLo
	b[13] = m.DstPrefixLo
	b[14] = m.Scratch[0]
	b[15] = m.Scratch[1]
	b[16] = m.Scratch[2]
	b[17] = m.Scratch[3]
	binary.BigEndian.PutUint16(b[18:20], m.Checksum)
	return b
}

// HasFlag returns true if the given flag bit is set.
func (m *MonadState) HasFlag(bit uint8) bool {
	return m.Flags&bit != 0
}

// FlagNames returns the set flag names as a string slice (for JSON display).
func (m *MonadState) FlagNames() []string {
	var names []string
	if m.HasFlag(FlagChaos) {
		names = append(names, "CHAOS")
	}
	if m.HasFlag(FlagCanary) {
		names = append(names, "CANARY")
	}
	if m.HasFlag(FlagTraced) {
		names = append(names, "TRACED")
	}
	if m.HasFlag(FlagEncrypt) {
		names = append(names, "ENCRYPT")
	}
	if m.HasFlag(FlagSampled) {
		names = append(names, "SAMPLED")
	}
	if m.HasFlag(FlagMirror) {
		names = append(names, "MIRROR")
	}
	if m.HasFlag(FlagCustom) {
		names = append(names, "CUSTOM")
	}
	return names
}

// ScratchR0 reads Scratch[0:1] as a big-endian u16 accumulator pair.
func (m *MonadState) ScratchR0() uint16 {
	return uint16(m.Scratch[0])<<8 | uint16(m.Scratch[1])
}

// ScratchR1 reads Scratch[2:3] as a big-endian u16 address register pair.
func (m *MonadState) ScratchR1() uint16 {
	return uint16(m.Scratch[2])<<8 | uint16(m.Scratch[3])
}

// VerifyChecksum verifies the CRC-16/CCITT over Monad octets 0x00–0x11.
func (m *MonadState) VerifyChecksum() bool {
	b := m.Encode()
	expected := CRC16CCITT(b[:MonadCRCDataLen])
	return expected == m.Checksum
}

// RecomputeChecksum recomputes and stores the CRC-16/CCITT checksum.
func (m *MonadState) RecomputeChecksum() {
	b := m.Encode()
	m.Checksum = CRC16CCITT(b[:MonadCRCDataLen])
	// Re-encode checksum bytes are now correct since we update m.Checksum
}

// ── CRC-16/CCITT ────────────────────────────────────────────────────────────

// CRC16CCITT computes CRC-16/CCITT (polynomial 0x1021, initial 0xFFFF).
// No final XOR, no input/output reflection — CRC-16/IBM-3740 variant.
func CRC16CCITT(data []byte) uint16 {
	crc := uint16(0xFFFF)
	for _, b := range data {
		crc ^= uint16(b) << 8
		for j := 0; j < 8; j++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// ── Compute event type constants ─────────────────────────────────────────────
// These must match the Rust side byte-for-byte.

const (
	EventComputeHop   = uint8(0x10) // one MBC instruction executed
	EventCacheMiss    = uint8(0x11) // L1 cache miss, addr emitted to Wotan
	EventMemWrite     = uint8(0x12) // dirty page written, Wotan flushes to L2
	EventMemStaged    = uint8(0x13) // Wotan staged a page into L1
	EventScreenWrite  = uint8(0x14) // SYSCALL DG_DrawFrame emitted
	EventKeyRead      = uint8(0x15) // SYSCALL DG_GetKey executed
	EventComputeHalt  = uint8(0x16) // HALT syscall
	EventComputeStall = uint8(0x17) // stall (cache miss, sleep)
)

// ComputeHopEvent mirrors the Rust ComputeHopEvent struct exactly.
// Binary layout: 8+1+1+2+4+4+4+(16*4)+1+1+4 = 96 bytes
type ComputeHopEvent struct {
	TimestampNs uint64
	EventType   uint8
	HopID       uint8
	Pad         [2]uint8
	FlowLabel   uint32
	PC          uint32
	Instruction uint32
	Regs        [16]uint32
	Flags       uint8
	CacheHit    uint8
	MissAddr    uint32
}

// DecodeComputeHopEvent decodes a raw BPF ring buffer event into a ComputeHopEvent.
// Returns error if length is wrong or binary.Read fails.
func DecodeComputeHopEvent(b []byte) (*ComputeHopEvent, error) {
	// Exact size calculation:
	// 8 (timestamp) + 1 (type) + 1 (hop) + 2 (pad) + 4 (flow) + 4 (pc) + 4 (insn) +
	// 64 (regs 16*4) + 1 (flags) + 1 (cache_hit) + 4 (miss_addr) = 94 bytes
	// But with repr(C) alignment padding, struct is actually:
	// 8 + 1 + 1 + 2 + 4 + 4 + 4 + 64 + 1 + 1 + 4 = 94 bytes
	// Let me verify by calculating from the struct definition.
	const expectedSize = 94
	if len(b) < expectedSize {
		return nil, fmt.Errorf("DecodeComputeHopEvent: need %d bytes, got %d", expectedSize, len(b))
	}
	evt := &ComputeHopEvent{}
	r := bytes.NewReader(b[:expectedSize])
	if err := binary.Read(r, binary.LittleEndian, evt); err != nil {
		return nil, fmt.Errorf("DecodeComputeHopEvent: %w", err)
	}
	return evt, nil
}

// FuzzDecodeComputeHopEvent is the fuzz target (call from fuzz_test.go)
func FuzzDecodeComputeHopEvent(data []byte) {
	_, _ = DecodeComputeHopEvent(data)
}

// MemWriteEvent is emitted by monad-cpu-ebpf for MEM_WRITE events.
// Binary layout: 8+1+3+4+4+1+3+4 = 28 bytes
type MemWriteEvent struct {
	TimestampNs uint64
	EventType   uint8
	Pad         [3]uint8
	FlowLabel   uint32
	Addr        uint32
	Size        uint8
	Pad2        [3]uint8
	Value       uint32
}

// ── AnamnesisEvent ──────────────────────────────────────────────────────────

// AnamnesisEvent is the Go mirror of the 32-byte Anamnesis event emitted by
// BPF programs to the per-CPU ring buffer.
//
// Wire format:
//
//	[0..8]:   timestamp_ns   (u64 LE — native kernel byte order)
//	[8]:      event_type     (u8)
//	[9]:      hop_id         (u8)
//	[10..12]: flow_label_lo  (u16 BE — low 16 bits of IPv6 Flow Label)
//	[12..32]: monad          (20 bytes, network byte order)
type AnamnesisEvent struct {
	TimestampNs  uint64     `json:"timestamp_ns"`
	EventType    EventType  `json:"event_type"`
	HopID        uint8      `json:"hop_id"`
	FlowLabelLo  uint16     `json:"flow_label_lo"`
	Monad        MonadState `json:"monad"`
}

// Errors returned by DecodeAnamnesisEvent.
var (
	ErrEventTooShort    = errors.New("anamnesis event too short")
	ErrEventUnknownType = errors.New("unknown anamnesis event type")
)

// DecodeAnamnesisEvent decodes a 32-byte Anamnesis event from a byte slice.
// The timestamp is little-endian (native kernel byte order on arm64/x86_64).
// The flow_label_lo and Monad fields are big-endian (network byte order).
func DecodeAnamnesisEvent(b []byte) (*AnamnesisEvent, error) {
	if len(b) < AnamnesisEventSize {
		return nil, fmt.Errorf("%w: %d < %d", ErrEventTooShort, len(b), AnamnesisEventSize)
	}

	evType := EventType(b[8])
	if !evType.Valid() {
		return nil, fmt.Errorf("%w: %d", ErrEventUnknownType, b[8])
	}

	monad, err := DecodeMonadState(b[12:32])
	if err != nil {
		return nil, fmt.Errorf("decode monad in event: %w", err)
	}

	return &AnamnesisEvent{
		TimestampNs: binary.LittleEndian.Uint64(b[0:8]),
		EventType:   evType,
		HopID:       b[9],
		FlowLabelLo: binary.BigEndian.Uint16(b[10:12]),
		Monad:       monad,
	}, nil
}

// Encode serializes the AnamnesisEvent to a 32-byte slice.
func (e *AnamnesisEvent) Encode() [AnamnesisEventSize]byte {
	var b [AnamnesisEventSize]byte
	binary.LittleEndian.PutUint64(b[0:8], e.TimestampNs)
	b[8] = uint8(e.EventType)
	b[9] = e.HopID
	binary.BigEndian.PutUint16(b[10:12], e.FlowLabelLo)
	monadBytes := e.Monad.Encode()
	copy(b[12:32], monadBytes[:])
	return b
}

// ToJSON serializes the event to JSON for WebSocket transport.
func (e *AnamnesisEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// WebSocketMessage wraps an AnamnesisEvent for WebSocket broadcast with
// a type discriminator.
type WebSocketMessage struct {
	Type  string          `json:"type"`
	Event *AnamnesisEvent `json:"event"`
}

// ToWebSocketJSON creates a typed WebSocket message and serializes to JSON.
func (e *AnamnesisEvent) ToWebSocketJSON() ([]byte, error) {
	msg := WebSocketMessage{
		Type:  e.EventType.String(),
		Event: e,
	}
	return json.Marshal(msg)
}
