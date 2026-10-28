// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package tlv provides Type-Length-Value encoding for Monad extensions.
// Uses pkg/protocol/encoding for the TLV wire format (RFC 8200 §4.2 option TLVs).
//
// Conformance reference for foundation-06 §"TLV Type Registry (Extended)"
// (docs/protocol/G8-REFERENCE-RULINGS.md): the type constants, the four
// reserved ranges and the critical bit match the registry, and
// TestRegistryParity parses the draft at test time. Not linked into any binary;
// no eBPF program parses TLVs beyond the Monad option, and Ring Path Counter
// (0x01) is emitted by no implementation (S-TRUE-WIRE Step 139).
package tlv

import (
	"fmt"

	"unheaded/pkg/protocol/encoding"
	perrors "unheaded/pkg/protocol/errors"
)

// TLVType represents the type identifier for a TLV element
type TLVType uint8

// Registered types (foundation-06 §"TLV Type Registry (Extended)", initial entries).
const (
	// RingPathCounter is optional, 4 bytes.
	RingPathCounter TLVType = 0x01
	// UnheadedMetricV1 is optional, 52 bytes (foundation-06 §UNHEADED_METRIC_V1).
	UnheadedMetricV1 TLVType = 0x2A

	// CriticalBit set means a receiver that does not know the type MUST
	// treat it as an error (UNKNOWN_CRITICAL_TLV, error code 0x07).
	CriticalBit TLVType = 0x80

	// MaxTLVsPerMonad is this implementation's limit, not a registry value.
	MaxTLVsPerMonad = 4
)

// Range is a reserved block of the type space (the critical bit aside).
type Range uint8

// Reserved ranges: 0x00-0x1F, 0x20-0x3F, 0x40-0x5F, 0x60-0x7F, each mirrored
// at 0x80-0xFF with the critical bit set.
const (
	RangeFoundation Range = iota
	RangeSophia
	RangeWotan
	RangeFuture
)

// String returns the registry's name for the range.
func (r Range) String() string {
	switch r {
	case RangeFoundation:
		return "Monad Foundation"
	case RangeSophia:
		return "Sophia Dictionary"
	case RangeWotan:
		return "Wotan Memory"
	default:
		return "Future Extensions"
	}
}

// TLV represents a Type-Length-Value element
type TLV struct {
	Type   TLVType
	Length uint8
	Value  []byte
}

// TLVBlock is a collection of TLV elements
type TLVBlock []TLV

// Entry is one registered type.
type Entry struct {
	Name     string
	Length   int // fixed value length in bytes
	Required bool
}

var registered = map[TLVType]Entry{
	RingPathCounter:  {Name: "Ring Path Counter", Length: 4},
	UnheadedMetricV1: {Name: "UNHEADED_METRIC_V1", Length: 52},
}

// Lookup returns the registry entry for a type.
func Lookup(t TLVType) (Entry, bool) {
	e, ok := registered[t]
	return e, ok
}

// IsCritical reports whether the critical bit (0x80) is set.
func (t TLVType) IsCritical() bool { return t&CriticalBit != 0 }

// Range returns the reserved range the type falls in.
func (t TLVType) Range() Range { return Range((t &^ CriticalBit) >> 5) }

// IsFoundation reports whether the type is in the Monad Foundation range
// (0x00-0x1F or its critical twin 0x80-0x9F).
func (t TLVType) IsFoundation() bool { return t.Range() == RangeFoundation }

// IsUnknownCritical reports whether a receiver must refuse the type.
func (t TLVType) IsUnknownCritical() bool {
	_, known := registered[t]
	return !known && t.IsCritical()
}

// ValidateRegistered applies the registry to one TLV: a registered type must
// carry its fixed length, an unknown critical type is UNKNOWN_CRITICAL_TLV,
// and an unknown non-critical type is skipped (nil).
func ValidateRegistered(tlv TLV) error {
	if len(tlv.Value) != int(tlv.Length) {
		return fmt.Errorf("TLV type 0x%02x: length mismatch (declared %d, actual %d)",
			uint8(tlv.Type), tlv.Length, len(tlv.Value))
	}
	if e, ok := registered[tlv.Type]; ok {
		if int(tlv.Length) != e.Length {
			return fmt.Errorf("TLV %s: length %d, registry says %d", e.Name, tlv.Length, e.Length)
		}
		return nil
	}
	if tlv.Type.IsCritical() {
		return perrors.New(perrors.UnknownCriticalTLV, fmt.Sprintf("TLV type 0x%02x", uint8(tlv.Type)))
	}
	return nil
}

// Parse parses a byte slice into a TLVBlock with validation.
// Uses pkg/protocol/encoding.DecodeTLV for wire format decoding.
func Parse(data []byte) (TLVBlock, error) {
	if len(data) == 0 {
		return TLVBlock{}, nil
	}

	var block TLVBlock
	offset := 0

	for offset < len(data) {
		// Check max TLVs per Monad limit
		if len(block) >= MaxTLVsPerMonad {
			return nil, fmt.Errorf("maximum %d TLVs per Monad exceeded", MaxTLVsPerMonad)
		}

		// Decode TLV using pkg/protocol/encoding.DecodeTLV
		encTLV, n, err := encoding.DecodeTLV(data[offset:])
		if err != nil {
			return nil, fmt.Errorf("failed to decode TLV at offset %d: %w", offset, err)
		}

		// Convert encoding.TLV to our local TLV type
		block = append(block, TLV{
			Type:   TLVType(encTLV.Type),
			Length: encTLV.Length,
			Value:  encTLV.Value,
		})

		offset += n
	}

	return block, nil
}

// Serialize converts a TLVBlock to bytes with validation.
// Uses pkg/protocol/encoding.EncodeTLV for wire format encoding.
func Serialize(block TLVBlock) ([]byte, error) {
	if len(block) > MaxTLVsPerMonad {
		return nil, fmt.Errorf("cannot serialize %d TLVs: maximum is %d per Monad",
			len(block), MaxTLVsPerMonad)
	}

	// Validate and encode each TLV
	var result []byte

	for _, tlv := range block {
		if len(tlv.Value) != int(tlv.Length) {
			return nil, fmt.Errorf("TLV type 0x%02x: length mismatch (declared %d, actual %d)",
				tlv.Type, tlv.Length, len(tlv.Value))
		}

		// Use pkg/protocol/encoding.EncodeTLV for wire format
		encoded, err := encoding.EncodeTLV(uint8(tlv.Type), tlv.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to encode TLV type 0x%02x: %w", tlv.Type, err)
		}

		result = append(result, encoded...)
	}

	return result, nil
}

// MustSerialize serializes a TLVBlock or panics on error
func MustSerialize(block TLVBlock) []byte {
	data, err := Serialize(block)
	if err != nil {
		panic(fmt.Sprintf("failed to serialize TLVBlock: %v", err))
	}
	return data
}

// NewTLV creates a new TLV with the given type and value
func NewTLV(tlvType TLVType, value []byte) (*TLV, error) {
	if len(value) > 255 {
		return nil, fmt.Errorf("TLV value too long: %d bytes (max 255)", len(value))
	}
	return &TLV{
		Type:   tlvType,
		Length: uint8(len(value)), // #nosec G115 -- bounded by the length of an already-allocated slice
		Value:  value,
	}, nil
}

// NewFoundationTLV creates a TLV whose type is in the Monad Foundation range.
func NewFoundationTLV(tlvType TLVType, value []byte) (*TLV, error) {
	if !tlvType.IsFoundation() {
		return nil, fmt.Errorf("type 0x%02x is not in the Monad Foundation range", uint8(tlvType))
	}
	return NewTLV(tlvType, value)
}

// Find retrieves the first TLV with the given type from the block
func (block TLVBlock) Find(tlvType TLVType) *TLV {
	for i := range block {
		if block[i].Type == tlvType {
			return &block[i]
		}
	}
	return nil
}

// FindAll retrieves all TLVs with the given type from the block
func (block TLVBlock) FindAll(tlvType TLVType) []TLV {
	var results []TLV
	for _, tlv := range block {
		if tlv.Type == tlvType {
			results = append(results, tlv)
		}
	}
	return results
}

// ValidateBlock performs comprehensive validation of a TLVBlock
func ValidateBlock(block TLVBlock) error {
	if len(block) > MaxTLVsPerMonad {
		return fmt.Errorf("TLVBlock contains %d TLVs, maximum is %d", len(block), MaxTLVsPerMonad)
	}

	for i, tlv := range block {
		if len(tlv.Value) != int(tlv.Length) {
			return fmt.Errorf("TLV at index %d: length mismatch (declared %d, actual %d)",
				i, tlv.Length, len(tlv.Value))
		}
		// Note: Length is u8, so > 255 is structurally impossible — that
		// check (SA4003) was removed. If TLV.Length ever widens beyond
		// uint8, restore an upper-bound check here.
	}

	return nil
}
