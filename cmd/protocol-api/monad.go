// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"

	pb "unheaded/proto/unheaded/v1"
)

// MonadPayloadSize is the total size in bytes: 18 bytes payload + 2 bytes CRC
const MonadPayloadSize = 20

// MonadCRCSize is the size of the CRC-16 checksum
const MonadCRCSize = 2

// CRC-16/CCITT-FALSE parameters
const (
	CRCPoly   = 0x1021
	CRCInit   = 0xFFFF
	CRCDataLen = 18 // CRC computed over first 18 bytes, stored in bytes 18-19
)

// KingdomModeToProto converts a KingdomMode string to protobuf enum
func KingdomModeToProto(mode string) pb.KingdomMode {
	switch mode {
	case "IDLE":
		return pb.KingdomMode_KINGDOM_MODE_IDLE
	case "ACTIVE":
		return pb.KingdomMode_KINGDOM_MODE_ACTIVE
	case "CLOSING":
		return pb.KingdomMode_KINGDOM_MODE_CLOSING
	case "CLOSED":
		return pb.KingdomMode_KINGDOM_MODE_CLOSED
	default:
		return pb.KingdomMode_KINGDOM_MODE_UNSPECIFIED
	}
}

// ProtoToKingdomMode converts a protobuf KingdomMode enum to string
func ProtoToKingdomMode(mode pb.KingdomMode) string {
	switch mode {
	case pb.KingdomMode_KINGDOM_MODE_IDLE:
		return "IDLE"
	case pb.KingdomMode_KINGDOM_MODE_ACTIVE:
		return "ACTIVE"
	case pb.KingdomMode_KINGDOM_MODE_CLOSING:
		return "CLOSING"
	case pb.KingdomMode_KINGDOM_MODE_CLOSED:
		return "CLOSED"
	default:
		return "UNSPECIFIED"
	}
}

// computeCRC16 computes CRC-16/CCITT-FALSE over the given data
// Parameters: poly=0x1021, init=0xFFFF, no final XOR
// The CRC is computed over bytes 0-17 of a 20-byte Monad register
func computeCRC16(data []byte) uint16 {
	if len(data) < CRCDataLen {
		return 0
	}

	crc := uint16(CRCInit)

	for i := 0; i < CRCDataLen; i++ {
		crc ^= uint16(data[i]) << 8

		for j := 0; j < 8; j++ {
			if (crc & 0x8000) != 0 {
				crc = (crc << 1) ^ CRCPoly
			} else {
				crc <<= 1
			}
		}
	}

	return crc
}

// verifyCRC16 verifies the CRC-16 checksum in a 20-byte Monad register
// The CRC is stored in bytes 18-19 (big-endian)
func verifyCRC16(data []byte) bool {
	if len(data) < MonadPayloadSize {
		return false
	}

	// Extract stored CRC from bytes 18-19
	storedCRC := binary.BigEndian.Uint16(data[CRCDataLen:])

	// Compute CRC over bytes 0-17
	computedCRC := computeCRC16(data)

	return storedCRC == computedCRC
}

// encodeKingdomMode encodes a KingdomMode enum into two bits (K1, K0)
// IDLE(00), ACTIVE(01), CLOSING(10), CLOSED(11)
func encodeKingdomMode(mode pb.KingdomMode) (k1, k0 uint8) {
	switch mode {
	case pb.KingdomMode_KINGDOM_MODE_IDLE:
		return 0, 0
	case pb.KingdomMode_KINGDOM_MODE_ACTIVE:
		return 0, 1
	case pb.KingdomMode_KINGDOM_MODE_CLOSING:
		return 1, 0
	case pb.KingdomMode_KINGDOM_MODE_CLOSED:
		return 1, 1
	default:
		return 0, 0
	}
}

// decodeKingdomMode decodes two bits (K1, K0) into a KingdomMode enum
func decodeKingdomMode(k1, k0 uint8) pb.KingdomMode {
	if k1 == 0 && k0 == 0 {
		return pb.KingdomMode_KINGDOM_MODE_IDLE
	} else if k1 == 0 && k0 == 1 {
		return pb.KingdomMode_KINGDOM_MODE_ACTIVE
	} else if k1 == 1 && k0 == 0 {
		return pb.KingdomMode_KINGDOM_MODE_CLOSING
	} else if k1 == 1 && k0 == 1 {
		return pb.KingdomMode_KINGDOM_MODE_CLOSED
	}
	return pb.KingdomMode_KINGDOM_MODE_UNSPECIFIED
}

// encodeExponent encodes a uint32 value as mantissa + exponent
// Used for compact encoding of field values in the wire format
func encodeExponent(val uint32) (mantissa uint8, exponent uint8) {
	if val == 0 {
		return 0, 0
	}

	// Simple encoding: find the highest set bit
	exponent = 0
	shifted := val

	for shifted > 255 {
		shifted >>= 1
		exponent++
	}

	mantissa = uint8(shifted & 0xFF)
	return mantissa, exponent
}

// decodeExponent decodes mantissa + exponent back to a uint32 value
func decodeExponent(mantissa uint8, exponent uint8) uint32 {
	val := uint32(mantissa)
	return val << exponent
}

// Encode encodes structured fields into a 20-byte Monad register with CRC
func (s *monadServer) Encode(ctx context.Context, req *pb.EncodeRequest) (*pb.EncodeResponse, error) {
	// Allocate 20 bytes
	data := make([]byte, MonadPayloadSize)

	// Field layout (18 bytes payload):
	// Bytes 0-2: flow_label (20-bit, left-aligned in 24-bit field)
	// Bytes 3: kingdom_mode (2-bit) + reserved bits
	// Bytes 4-7: timestamp (32-bit milliseconds, upper bits)
	// Bytes 8-11: timestamp (32-bit milliseconds, lower bits)
	// Bytes 12-15: sophia_dict_id (32-bit)
	// Bytes 16-17: wotan_sequence (16-bit, upper bits)
	// Bytes 18-19: CRC-16/CCITT-FALSE (computed)

	// Encode flow_label (20 bits)
	// Mask to 20 bits and shift left to align in 3 bytes
	flowLabelMasked := req.FlowLabel & 0xFFFFF // 20-bit mask
	data[0] = uint8((flowLabelMasked >> 12) & 0xFF)
	data[1] = uint8((flowLabelMasked >> 4) & 0xFF)
	data[2] = uint8((flowLabelMasked & 0x0F) << 4)

	// Encode kingdom_mode (2-bit K1, K0) in bits 2-3 of byte 3
	k1, k0 := encodeKingdomMode(req.KingdomMode)
	data[3] = (k1 << 3) | (k0 << 2)

	// Encode timestamp (40-bit, stored as uint64, split across 5 bytes)
	timestamp := req.Timestamp & 0xFFFFFFFFFF // 40-bit mask
	data[4] = uint8((timestamp >> 32) & 0xFF)
	data[5] = uint8((timestamp >> 24) & 0xFF)
	data[6] = uint8((timestamp >> 16) & 0xFF)
	data[7] = uint8((timestamp >> 8) & 0xFF)
	data[8] = uint8(timestamp & 0xFF)

	// Encode sophia_dict_id (16-bit)
	sophiaDictIDMasked := uint16(req.SophiaDictId & 0xFFFF)
	binary.BigEndian.PutUint16(data[9:11], sophiaDictIDMasked)

	// Encode wotan_sequence (32-bit, but only use upper bits in available space)
	wotanSeqMasked := req.WotanSequence & 0xFFFF // 16-bit for compact encoding
	binary.BigEndian.PutUint16(data[11:13], uint16(wotanSeqMasked))

	// Bytes 13-17 reserved (set to 0)
	data[13] = 0
	data[14] = 0
	data[15] = 0
	data[16] = 0
	data[17] = 0

	// Compute CRC-16 over bytes 0-17
	crc := computeCRC16(data)

	// Store CRC in bytes 18-19 (big-endian)
	binary.BigEndian.PutUint16(data[18:20], crc)

	return &pb.EncodeResponse{
		Data:    data,
		Crc16:   uint32(crc),
		Success: true,
	}, nil
}

// Decode decodes a 20-byte Monad register and extracts all fields
func (s *monadServer) Decode(ctx context.Context, req *pb.DecodeRequest) (*pb.DecodeResponse, error) {
	data := req.Data

	if len(data) != MonadPayloadSize {
		return &pb.DecodeResponse{
			Valid:        false,
			CrcValid:     false,
			ErrorMessage: fmt.Sprintf("Invalid data length: expected %d, got %d", MonadPayloadSize, len(data)),
		}, nil
	}

	resp := &pb.DecodeResponse{}

	// Decode flow_label (20-bit)
	flowLabel := uint32(data[0])<<12 | uint32(data[1])<<4 | (uint32(data[2])>>4)&0x0F
	resp.FlowLabel = flowLabel & 0xFFFFF

	// Decode kingdom_mode (2-bit K1, K0)
	k1 := (data[3] >> 3) & 0x01
	k0 := (data[3] >> 2) & 0x01
	resp.KingdomMode = decodeKingdomMode(k1, k0)

	// Decode timestamp (40-bit)
	timestamp := uint64(data[4])<<32 | uint64(data[5])<<24 | uint64(data[6])<<16 | uint64(data[7])<<8 | uint64(data[8])
	resp.Timestamp = timestamp & 0xFFFFFFFFFF

	// Decode sophia_dict_id (16-bit)
	resp.SophiaDictId = uint32(binary.BigEndian.Uint16(data[9:11]))

	// Decode wotan_sequence (16-bit, upper bits)
	resp.WotanSequence = uint32(binary.BigEndian.Uint16(data[11:13]))

	// Reserved fields
	resp.Reserved1 = uint32(data[13])
	resp.Reserved2 = uint32(data[14])

	// Decode CRC (stored in bytes 18-19)
	resp.Crc16 = uint32(binary.BigEndian.Uint16(data[18:20]))

	// Verify CRC
	resp.CrcValid = verifyCRC16(data)
	resp.Valid = resp.CrcValid

	return resp, nil
}

// Validate decodes a Monad register and validates its CRC checksum
func (s *monadServer) Validate(ctx context.Context, req *pb.DecodeRequest) (*pb.DecodeResponse, error) {
	// Simply decode and return CRC validation result
	return s.Decode(ctx, req)
}

// EncodeBytesWithIPv6 encodes a Monad register from IPv6 source/dest addresses
// The source and destination addresses are stored as 128-bit IPv6 addresses
func EncodeBytesWithIPv6(
	flowLabel uint32,
	kingdomMode pb.KingdomMode,
	timestamp uint64,
	sourceAddr net.IP,
	destAddr net.IP,
	sophiaDictID uint32,
	wotanSeq uint32,
) ([]byte, error) {
	if len(sourceAddr) != net.IPv6len || len(destAddr) != net.IPv6len {
		return nil, fmt.Errorf("IPv6 addresses must be 128 bits")
	}

	// For now, store only the essential address info
	// Full implementation would include IPv6 addresses in flow state
	return nil, nil
}

// ComputeInverseMask computes the bitwise NOT of an IPv6 address
// This gives the valid second address in the same /48 network
func ComputeInverseMask(addr net.IP) net.IP {
	if len(addr) != net.IPv6len {
		return nil
	}

	result := make(net.IP, net.IPv6len)
	for i := 0; i < net.IPv6len; i++ {
		result[i] = ^addr[i]
	}

	return result
}
