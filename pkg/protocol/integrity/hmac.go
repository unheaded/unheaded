package integrity

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"net"
)

// HMACTagSize is the size of the HMAC tag (8 bytes = 64 bits)
const HMACTagSize = 8

// HMACComputer computes and verifies HMAC-SHA256 truncated to 8 bytes
type HMACComputer struct {
	flowSecret [32]byte // Per-flow secret derived from node secret
}

// NewHMACComputer creates a new HMAC computer with a derived flow secret
func NewHMACComputer(flowSecret [32]byte) *HMACComputer {
	return &HMACComputer{
		flowSecret: flowSecret,
	}
}

// DeriveFlowSecret derives a per-flow secret using HKDF-SHA256
// Parameters: nodeSecret, flowLabel, srcIP, dstIP
func DeriveFlowSecret(nodeSecret []byte, flowLabel uint32, srcIP net.IP, dstIP net.IP) ([32]byte, error) {
	if nodeSecret == nil || len(nodeSecret) == 0 {
		return [32]byte{}, fmt.Errorf("node secret cannot be nil or empty")
	}

	if len(srcIP) == 0 {
		return [32]byte{}, fmt.Errorf("source IP cannot be empty")
	}

	if len(dstIP) == 0 {
		return [32]byte{}, fmt.Errorf("destination IP cannot be empty")
	}

	// Use HMAC-SHA256 as a simple KDF
	// Context: nodeSecret || flowLabel || srcIP || dstIP
	h := hmac.New(sha256.New, nodeSecret)

	// Write flow label (big-endian)
	h.Write([]byte{
		byte(flowLabel >> 24),
		byte(flowLabel >> 16),
		byte(flowLabel >> 8),
		byte(flowLabel),
	})

	// Write addresses
	h.Write(srcIP)
	h.Write(dstIP)

	var secret [32]byte
	digest := h.Sum(nil)
	copy(secret[:], digest[:32])

	return secret, nil
}

// Compute calculates the HMAC-SHA256 tag for a monad header (18 bytes)
// Returns the tag as [8]byte (truncated from SHA256)
func (hc *HMACComputer) Compute(monadHeader [18]byte) [8]byte {
	h := hmac.New(sha256.New, hc.flowSecret[:])
	h.Write(monadHeader[:])
	digest := h.Sum(nil)

	var tag [8]byte
	copy(tag[:], digest[:HMACTagSize])
	return tag
}

// Verify checks if the provided tag matches the computed tag using constant-time comparison
// Returns true if the tag is valid, false otherwise
func (hc *HMACComputer) Verify(monadHeader [18]byte, tag [8]byte) bool {
	computed := hc.Compute(monadHeader)
	return hmac.Equal(computed[:], tag[:])
}

// CRC16CCITT computes CRC-16-CCITT polynomial x^16 + x^12 + x^5 + 1
// Kept for backward compatibility
func CRC16CCITT(data []byte) uint16 {
	if data == nil {
		return 0
	}

	crc := uint16(0xffff)

	for _, b := range data {
		crc ^= uint16(b) << 8

		for i := 0; i < 8; i++ {
			crc <<= 1
			if (crc & 0x10000) != 0 {
				crc ^= 0x1021
				crc &= 0xffff
			}
		}
	}

	return crc ^ 0xffff
}

// ComputeCRC16CCITT is a convenience function for backward compatibility
func ComputeCRC16CCITT(data []byte) uint16 {
	return CRC16CCITT(data)
}

// VerifyCRC16CCITT verifies a CRC-16-CCITT checksum
// Kept for backward compatibility
func VerifyCRC16CCITT(data []byte, expectedCRC uint16) bool {
	if data == nil {
		return expectedCRC == 0
	}
	return CRC16CCITT(data) == expectedCRC
}
