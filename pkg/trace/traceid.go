// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package trace provides 128-bit trace ID generation for the Unheaded platform.
//
// Remediation for Lich D4 findings: 20-bit IPv6 flow labels have ~40% collision
// probability at 1000 concurrent flows (birthday attack). Production traces MUST
// use 128-bit IDs for collision resistance.
//
// Design:
//   - UUIDv7 (RFC 9562) provides time-ordered, k-sortable 128-bit IDs
//   - Flow labels are used ONLY as BPF map lookup hints, NOT as trace identity
//   - All Wotan messages carry the full 128-bit trace ID
//   - Collision probability at 10^18 concurrent flows: ~1.5e-20
package trace

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// TraceID is a 128-bit unique identifier for distributed tracing.
// Uses UUIDv7 format: time-ordered, k-sortable, 122 bits of entropy + timestamp.
type TraceID [16]byte

// NewTraceID generates a new 128-bit trace ID using UUIDv7.
// UUIDv7 is time-ordered (k-sortable): IDs generated later sort after earlier ones.
// This is critical for efficient log correlation and time-range queries.
func NewTraceID() (TraceID, error) {
	u, err := uuid.NewV7()
	if err != nil {
		return TraceID{}, fmt.Errorf("generate UUIDv7: %w", err)
	}
	var id TraceID
	copy(id[:], u[:])
	return id, nil
}

// MustNewTraceID generates a new trace ID or panics.
// Use only in initialization code where failure is unrecoverable.
func MustNewTraceID() TraceID {
	id, err := NewTraceID()
	if err != nil {
		panic(fmt.Sprintf("trace: failed to generate ID: %v", err))
	}
	return id
}

// TraceIDFromBytes reconstructs a TraceID from a 16-byte slice.
// Returns an error if the slice is not exactly 16 bytes.
func TraceIDFromBytes(b []byte) (TraceID, error) {
	if len(b) != 16 {
		return TraceID{}, fmt.Errorf("trace ID must be 16 bytes, got %d", len(b))
	}
	var id TraceID
	copy(id[:], b)
	return id, nil
}

// TraceIDFromHex parses a 32-character hex string into a TraceID.
// Accepts both "abcdef01..." and "abcdef01-..." (UUID-style with dashes).
func TraceIDFromHex(s string) (TraceID, error) {
	// Strip dashes for UUID-style input
	s = strings.ReplaceAll(s, "-", "")
	if len(s) != 32 {
		return TraceID{}, fmt.Errorf("trace ID hex must be 32 chars, got %d", len(s))
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return TraceID{}, fmt.Errorf("invalid hex in trace ID: %w", err)
	}
	var id TraceID
	copy(id[:], b)
	return id, nil
}

// FlowLabelHint extracts a 20-bit flow label hint from the trace ID.
// This is used ONLY as a BPF map lookup key, NOT as the trace identity.
//
// Security note (D4-001): The flow label is a lossy projection of the 128-bit ID.
// Multiple trace IDs may map to the same flow label. The full 128-bit ID must
// be carried in Wotan messages for correct trace correlation.
func (id TraceID) FlowLabelHint() uint32 {
	// Use bytes 12-15 (last 4 bytes) which contain random bits in UUIDv7
	v := binary.BigEndian.Uint32(id[12:16])
	// Mask to 20 bits (IPv6 flow label size)
	return v & 0x000FFFFF
}

// TraceIDFromFlowLabel creates a lookup key from a flow label.
// This does NOT reconstruct the original trace ID (that's impossible from 20 bits).
// It creates a canonical lookup key for BPF map correlation.
//
// The flow label is stored in the lower 20 bits of the last 4 bytes,
// matching the format returned by FlowLabelHint().
func TraceIDFromFlowLabel(label uint32) FlowLabelKey {
	return FlowLabelKey(label & 0x000FFFFF)
}

// FlowLabelKey is a 20-bit BPF map lookup key derived from a flow label.
// It is NOT a trace ID -- multiple traces may share the same flow label key.
type FlowLabelKey uint32

// String returns the flow label as a zero-padded 5-character hex string.
func (k FlowLabelKey) String() string {
	return fmt.Sprintf("%05x", uint32(k))
}

// Bytes returns the flow label as a 4-byte big-endian slice (for BPF map lookups).
func (k FlowLabelKey) Bytes() []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(k))
	return b
}

// String returns the trace ID as a 32-character lowercase hex string.
func (id TraceID) String() string {
	return hex.EncodeToString(id[:])
}

// Bytes returns the raw 16-byte representation.
func (id TraceID) Bytes() []byte {
	b := make([]byte, 16)
	copy(b, id[:])
	return b
}

// IsZero returns true if the trace ID is all zeros (uninitialized).
func (id TraceID) IsZero() bool {
	return id == TraceID{}
}

// W3CTraceParent returns the trace ID formatted for W3C Trace Context propagation.
// Format: "00-{trace-id}-{span-id}-{flags}"
// The span ID is derived from the trace ID (bytes 8-15) for simplicity.
func (id TraceID) W3CTraceParent(spanID [8]byte, sampled bool) string {
	flags := "00"
	if sampled {
		flags = "01"
	}
	return fmt.Sprintf("00-%s-%s-%s",
		hex.EncodeToString(id[:]),
		hex.EncodeToString(spanID[:]),
		flags,
	)
}

// TimestampMS extracts the Unix timestamp in milliseconds from a UUIDv7 trace ID.
// UUIDv7 stores a 48-bit millisecond timestamp in the first 6 bytes.
func (id TraceID) TimestampMS() int64 {
	// UUIDv7: bytes 0-5 are the 48-bit Unix timestamp in milliseconds
	var ts int64
	ts |= int64(id[0]) << 40
	ts |= int64(id[1]) << 32
	ts |= int64(id[2]) << 24
	ts |= int64(id[3]) << 16
	ts |= int64(id[4]) << 8
	ts |= int64(id[5])
	return ts
}
