// Package tlv provides Type-Length-Value encoding for Monad extensions.
// Uses pkg/protocol/encoding for TLV wire format per RFC 9114 §7 and RFC 8200 §4.
package tlv

import (
	"fmt"

	"unheaded/pkg/protocol/encoding"
)

// TLVType represents the type identifier for a TLV element
type TLVType uint8

// TLV type ranges as specified by H3 finding
const (
	// Core types
	PRIORITY_HINT    TLVType = 0x01
	TRACE_SPAN_ID    TLVType = 0x02
	ERROR_CODE       TLVType = 0x03
	FLOW_SEQUENCE    TLVType = 0x04
	RING_PATH_COUNT  TLVType = 0x05
	HMAC_TAG         TLVType = 0x06

	// Type range boundaries
	CoreMin       TLVType = 0x00
	CoreMax       TLVType = 0x0f
	NegotiatedMin TLVType = 0x10
	NegotiatedMax TLVType = 0x7f
	PaddingMin    TLVType = 0x80
	PaddingMax    TLVType = 0xff

	// Maximum TLVs per Monad
	MaxTLVsPerMonad = 4
)

// TLV represents a Type-Length-Value element
type TLV struct {
	Type   TLVType
	Length uint8
	Value  []byte
}

// TLVBlock is a collection of TLV elements
type TLVBlock []TLV

// IsCore checks if a type is a core type
func (t TLVType) IsCore() bool {
	return t >= CoreMin && t <= CoreMax
}

// IsNegotiated checks if a type is a negotiated extension type
func (t TLVType) IsNegotiated() bool {
	return t >= NegotiatedMin && t <= NegotiatedMax
}

// IsPadding checks if a type is a padding type
func (t TLVType) IsPadding() bool {
	return t >= PaddingMin && t <= PaddingMax
}

// IsGreasing checks if a type is a greasing type (reserved for version negotiation)
// Greasing types follow pattern: 0x1F*N+0x21
func (t TLVType) IsGreasing() bool {
	if t < NegotiatedMin || t > NegotiatedMax {
		return false
	}
	// Check if it matches pattern 0x1F*N+0x21
	val := uint16(t)
	if (val - 0x21) == 0 {
		return true
	}
	if (val - 0x21)%0x1f == 0 && (val-0x21)/0x1f < 4 {
		return true
	}
	return false
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
		Length: uint8(len(value)),
		Value:  value,
	}, nil
}

// NewCoreTLV creates a core TLV with validation
func NewCoreTLV(tlvType TLVType, value []byte) (*TLV, error) {
	if !tlvType.IsCore() {
		return nil, fmt.Errorf("type 0x%02x is not a core type", tlvType)
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

		if tlv.Length > 255 {
			return fmt.Errorf("TLV at index %d: value too long (%d bytes)", i, tlv.Length)
		}
	}

	return nil
}
