// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package encoding provides shared wire encoding primitives for the Unheaded protocol.
//
// All protocol packages MUST use these functions for encoding/decoding rather than
// reimplementing encoding logic. This ensures consistency with RFC specifications
// and enables centralized hardening.
//
// Wire encoding patterns implemented:
//   - Variable-length integers (RFC 9000 §16)
//   - Exponent encoding (Monad wire format spec)
//   - CRC-16/CCITT-FALSE (Monad checksum)
//   - CRC-32/MPEG-2 (extended checksum)
//   - TLV encoding (RFC 9114 §7 / RFC 8200 §4)
//
// All functions are safe for concurrent use. All multi-byte values use
// network byte order (big-endian) per RFC 791 and RFC 8200.
package encoding

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/bits"
)

// ErrVarintTooLarge is returned when a varint value exceeds 2^62-1.
// Per RFC 9000 §16, variable-length integers cannot exceed this value.
var ErrVarintTooLarge = errors.New("encoding: varint value exceeds 2^62-1 (RFC 9000 §16)")

// ErrVarintTruncated is returned when the input buffer is too short
// to contain a complete varint.
var ErrVarintTruncated = errors.New("encoding: truncated varint input")

// ErrExponentOverflow is returned when exponent decoding would overflow uint64.
var ErrExponentOverflow = errors.New("encoding: exponent decode overflow")

// ErrTLVTruncated is returned when a TLV is truncated.
var ErrTLVTruncated = errors.New("encoding: truncated TLV")

// ErrTLVValueTooLarge is returned when a TLV value exceeds max length.
var ErrTLVValueTooLarge = errors.New("encoding: TLV value exceeds maximum length")

// MaxVarint is the maximum value encodable in a QUIC variable-length integer.
// Per RFC 9000 §16: 2^62 - 1 = 4611686018427387903.
const MaxVarint = uint64(1<<62 - 1)

// MaxTLVValueLen is the maximum length of a TLV value field (255 bytes).
const MaxTLVValueLen = 255

// EncodeVarint encodes a uint64 value as a QUIC variable-length integer.
//
// Per RFC 9000 §16, the encoding uses 1, 2, 4, or 8 bytes depending on
// the value magnitude. The two most significant bits of the first byte
// encode the length:
//
//	00 = 1 byte  (6-bit value,  max 63)
//	01 = 2 bytes (14-bit value, max 16383)
//	10 = 4 bytes (30-bit value, max 1073741823)
//	11 = 8 bytes (62-bit value, max 4611686018427387903)
//
// Returns nil and ErrVarintTooLarge if v > MaxVarint.
func EncodeVarint(v uint64) ([]byte, error) {
	if v > MaxVarint {
		return nil, ErrVarintTooLarge
	}

	switch {
	case v <= 63:
		return []byte{byte(v)}, nil
	case v <= 16383:
		buf := make([]byte, 2)
		binary.BigEndian.PutUint16(buf, uint16(v)|0x4000)
		return buf, nil
	case v <= 1073741823:
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, uint32(v)|0x80000000)
		return buf, nil
	default:
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, v|0xC000000000000000)
		return buf, nil
	}
}

// DecodeVarint decodes a QUIC variable-length integer from the given buffer.
//
// Returns the decoded value and the number of bytes consumed.
// Returns ErrVarintTruncated if the buffer is too short.
//
// Per RFC 9000 §16: "The QUIC variable-length integer encoding reserves
// the two most significant bits of the first byte to encode the base 2
// logarithm of the integer encoding length in bytes."
func DecodeVarint(b []byte) (uint64, int, error) {
	if len(b) == 0 {
		return 0, 0, ErrVarintTruncated
	}

	prefix := b[0] >> 6
	length := 1 << prefix

	if len(b) < length {
		return 0, 0, ErrVarintTruncated
	}

	switch length {
	case 1:
		return uint64(b[0] & 0x3F), 1, nil
	case 2:
		return uint64(binary.BigEndian.Uint16(b[:2]) & 0x3FFF), 2, nil
	case 4:
		return uint64(binary.BigEndian.Uint32(b[:4]) & 0x3FFFFFFF), 4, nil
	case 8:
		return binary.BigEndian.Uint64(b[:8]) & 0x3FFFFFFFFFFFFFFF, 8, nil
	default:
		// Unreachable: prefix is 2 bits, length is always 1, 2, 4, or 8.
		panic("unreachable: invalid varint prefix")
	}
}

// EncodeExponent encodes a uint32 value using Monad exponent encoding.
//
// The encoding stores a mantissa (4 bits) and exponent (8 bits) in a
// 16-bit field, allowing compact representation of values spanning
// many orders of magnitude.
//
// Encoding: value = mantissa × 2^(exponent-1) for exponent > 0.
// Special case: exponent == 0 means value == 0.
//
// The returned uint16 is in big-endian wire format:
//
//	bits [15:8] = exponent
//	bits [7:4]  = mantissa
//	bits [3:0]  = reserved (zero)
func EncodeExponent(value uint32) uint16 {
	if value == 0 {
		return 0
	}

	// Find highest set bit to determine exponent.
	highBit := 32 - bits.LeadingZeros32(value)
	exp := uint8(highBit)

	// Extract 4-bit mantissa from the most significant bits.
	var mantissa uint8
	if highBit > 4 {
		mantissa = uint8(value >> (highBit - 4))
	} else {
		mantissa = uint8(value << (4 - highBit))
	}
	mantissa &= 0x0F

	return uint16(exp)<<8 | uint16(mantissa)<<4
}

// DecodeExponent decodes a Monad exponent-encoded uint16 to its uint64 value.
//
// Returns ErrExponentOverflow if the decoded value would exceed uint64.
// This is a security-critical check: crafted exponent values can overflow.
//
// The mantissa captures the top 4 significant bits of the original value,
// and the exponent records the bit-width. Reconstruction is:
//
//	if exp >= 4: value = mantissa << (exp - 4)
//	if exp <  4: value = mantissa >> (4 - exp)
//
// See: Black Mage Dark Grimoire — Monad Value field attack vectors.
func DecodeExponent(encoded uint16) (uint64, error) {
	exp := uint8(encoded >> 8)
	mantissa := uint8((encoded >> 4) & 0x0F)

	if exp == 0 {
		return 0, nil
	}

	if exp < 4 {
		// Small values: right-shift mantissa to recover original magnitude.
		return uint64(mantissa) >> (4 - exp), nil
	}

	shift := uint(exp - 4)

	// Overflow check: mantissa << shift must fit in uint64.
	if shift >= 64 {
		return 0, ErrExponentOverflow
	}
	if shift > 0 {
		maxMantissa := uint64(^uint64(0)) >> shift
		if uint64(mantissa) > maxMantissa {
			return 0, ErrExponentOverflow
		}
	}

	return uint64(mantissa) << shift, nil
}

// CRC16CCITT computes CRC-16/CCITT-FALSE over the given data.
//
// Parameters per Monad wire format spec:
//
//	Polynomial: 0x1021
//	Initial:    0xFFFF
//	Reflect In: false
//	Reflect Out: false
//	Final XOR:  0x0000
//
// This is NOT a cryptographic hash. Per Security Considerations:
// CRC-16 detects accidental corruption only. Use HMAC (integrity package)
// for authentication.
func CRC16CCITT(data []byte) uint16 {
	crc := uint16(0xFFFF)
	for _, b := range data {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// CRC32MPEG2 computes CRC-32/MPEG-2 over the given data.
//
// Parameters per Monad extended checksum spec:
//
//	Polynomial: 0x04C11DB7
//	Initial:    0xFFFFFFFF
//	Reflect In: false
//	Reflect Out: false
//	Final XOR:  0x00000000
func CRC32MPEG2(data []byte) uint32 {
	crc := uint32(0xFFFFFFFF)
	for _, b := range data {
		crc ^= uint32(b) << 24
		for i := 0; i < 8; i++ {
			if crc&0x80000000 != 0 {
				crc = (crc << 1) ^ 0x04C11DB7
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// TLV represents a Type-Length-Value encoded field.
//
// Per RFC 9114 §7 and RFC 8200 §4.2, TLV encoding provides extensible
// field encoding for protocol options. The Unheaded protocol uses TLV
// for Monad extension options.
type TLV struct {
	Type   uint8
	Length uint8
	Value  []byte
}

// EncodeTLV encodes a TLV triplet to wire format.
//
// Wire layout:
//
//	+--------+--------+--------...--------+
//	|  Type  | Length |     Value          |
//	+--------+--------+--------...--------+
//	 1 byte   1 byte   Length bytes
//
// Returns ErrTLVValueTooLarge if len(value) > MaxTLVValueLen.
func EncodeTLV(typ uint8, value []byte) ([]byte, error) {
	if len(value) > MaxTLVValueLen {
		return nil, fmt.Errorf("%w: got %d, max %d", ErrTLVValueTooLarge, len(value), MaxTLVValueLen)
	}

	buf := make([]byte, 2+len(value))
	buf[0] = typ
	buf[1] = uint8(len(value))
	copy(buf[2:], value)
	return buf, nil
}

// DecodeTLV decodes a TLV from the given buffer.
//
// Returns the decoded TLV and the number of bytes consumed.
// Returns ErrTLVTruncated if the buffer is too short.
func DecodeTLV(b []byte) (TLV, int, error) {
	if len(b) < 2 {
		return TLV{}, 0, ErrTLVTruncated
	}

	typ := b[0]
	length := b[1]
	total := 2 + int(length)

	if len(b) < total {
		return TLV{}, 0, ErrTLVTruncated
	}

	value := make([]byte, length)
	copy(value, b[2:total])

	return TLV{Type: typ, Length: length, Value: value}, total, nil
}

// DecodeTLVSequence decodes a sequence of TLVs from the buffer.
//
// Returns all decoded TLVs and the total bytes consumed.
// Stops at the first error or end of buffer.
func DecodeTLVSequence(b []byte) ([]TLV, int, error) {
	var tlvs []TLV
	offset := 0

	for offset < len(b) {
		tlv, n, err := DecodeTLV(b[offset:])
		if err != nil {
			return tlvs, offset, err
		}
		tlvs = append(tlvs, tlv)
		offset += n
	}

	return tlvs, offset, nil
}
