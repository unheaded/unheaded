// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// LICH-003: CRC-16 Collision Attack Fuzzer
//
// Target: pkg/protocol/encoding.CRC16CCITT
// Goal: Systematically hunt for CRC-16 collision attacks.
//
// CRC-16 has only 65,536 possible values, so collisions are inevitable.
// This harness specifically looks for:
//   - Pairs of inputs that produce the same CRC (collision pairs)
//   - Single-bit modifications that preserve the CRC (stealth modifications)
//   - Length-extension attacks (appending data that preserves CRC)
//   - Monad-wire-specific collisions (18-byte headers that share a CRC)
//
// Per the Monad wire format spec: CRC-16 detects accidental corruption only.
// HMAC (pkg/protocol/integrity) is required for authentication.
// This harness validates that CRC-16 does NOT provide cryptographic guarantees.

package harnesses

import (
	"encoding/binary"
	"testing"

	"unheaded/pkg/protocol/encoding"
)

// FuzzCRC16CollisionPair searches for two distinct inputs with the same CRC.
func FuzzCRC16CollisionPair(f *testing.F) {
	// Pairs designed to be close but different
	f.Add([]byte("Hello, World!"), []byte("Hello, World?"))
	f.Add([]byte{0x00, 0x00, 0x00, 0x00}, []byte{0x00, 0x00, 0x00, 0x01})
	f.Add([]byte{0xFF}, []byte{0xFE})
	f.Add([]byte{0xAA, 0x55}, []byte{0x55, 0xAA})

	// Same-length inputs with different content
	f.Add(make([]byte, 18), append(make([]byte, 17), 0x01))

	// Monad-sized headers (18 bytes)
	header1 := make([]byte, 18)
	header1[0] = 0x01 // version
	header2 := make([]byte, 18)
	header2[0] = 0x01
	header2[1] = 0x01 // different flags
	f.Add(header1, header2)

	f.Fuzz(func(t *testing.T, a, b []byte) {
		if len(a) == 0 || len(b) == 0 {
			return
		}

		crcA := encoding.CRC16CCITT(a)
		crcB := encoding.CRC16CCITT(b)

		if crcA == crcB && string(a) != string(b) {
			// Found a collision -- log it for the campaign report.
			// This is EXPECTED for CRC-16 (birthday bound ~256 inputs).
			// It validates that CRC-16 is NOT suitable for authentication.
			t.Logf("LICH-003 COLLISION: CRC=0x%04X, len(a)=%d, len(b)=%d, a[0]=0x%02X, b[0]=0x%02X",
				crcA, len(a), len(b), a[0], b[0])
		}
	})
}

// FuzzCRC16StealthBitFlip searches for single-bit modifications that preserve CRC.
// This is the most dangerous attack: an adversary can modify wire data without
// detection if they find a bit-flip that preserves the checksum.
func FuzzCRC16StealthBitFlip(f *testing.F) {
	// Valid Monad header (18 bytes)
	header := make([]byte, 18)
	header[0] = 0x01 // version
	header[1] = 0x00 // flags
	binary.BigEndian.PutUint32(header[2:6], 0x00010001)  // flow label
	binary.BigEndian.PutUint16(header[6:8], 0x0420)       // value
	binary.BigEndian.PutUint32(header[8:12], 0x00000001)  // namespace
	binary.BigEndian.PutUint32(header[12:16], 0x00000001) // sequence
	binary.BigEndian.PutUint16(header[16:18], 64)         // TTL
	f.Add(header)

	// Various payload sizes
	f.Add([]byte{0x00})
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	f.Add(make([]byte, 100))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 {
			return
		}

		originalCRC := encoding.CRC16CCITT(data)

		// Try every single-bit flip
		for byteIdx := 0; byteIdx < len(data); byteIdx++ {
			for bitIdx := 0; bitIdx < 8; bitIdx++ {
				modified := make([]byte, len(data))
				copy(modified, data)
				modified[byteIdx] ^= 1 << uint(bitIdx)

				modifiedCRC := encoding.CRC16CCITT(modified)
				if modifiedCRC == originalCRC {
					// Stealth modification found!
					// This is EXPECTED for CRC-16 -- it has limited error detection.
					// Log for security audit.
					t.Logf("LICH-003 STEALTH: single-bit flip at byte %d bit %d preserves CRC 0x%04X (len=%d)",
						byteIdx, bitIdx, originalCRC, len(data))
				}
			}
		}

		// Try two-bit flips (compensating errors)
		if len(data) >= 2 {
			for i := 0; i < len(data) && i < 4; i++ {
				for j := i + 1; j < len(data) && j < 6; j++ {
					modified := make([]byte, len(data))
					copy(modified, data)
					modified[i] ^= 0x01
					modified[j] ^= 0x01

					modifiedCRC := encoding.CRC16CCITT(modified)
					if modifiedCRC == originalCRC {
						t.Logf("LICH-003 STEALTH-2BIT: compensating flips at bytes %d,%d preserve CRC 0x%04X",
							i, j, originalCRC)
					}
				}
			}
		}
	})
}

// FuzzCRC16MonadCollision specifically targets 18-byte Monad headers.
// Searches for two valid-looking Monad headers that produce the same CRC.
func FuzzCRC16MonadCollision(f *testing.F) {
	f.Add(
		uint8(0x01), uint8(0x00), uint32(1), uint16(0x0420), uint32(1), uint32(1), uint16(64),
		uint8(0x01), uint8(0x01), uint32(2), uint16(0x0420), uint32(1), uint32(2), uint16(128),
	)

	f.Fuzz(func(t *testing.T,
		v1 uint8, f1 uint8, fl1 uint32, val1 uint16, ns1 uint32, seq1 uint32, ttl1 uint16,
		v2 uint8, f2 uint8, fl2 uint32, val2 uint16, ns2 uint32, seq2 uint32, ttl2 uint16,
	) {
		// Build header 1
		h1 := make([]byte, 18)
		h1[0] = v1
		h1[1] = f1
		binary.BigEndian.PutUint32(h1[2:6], fl1)
		binary.BigEndian.PutUint16(h1[6:8], val1)
		binary.BigEndian.PutUint32(h1[8:12], ns1)
		binary.BigEndian.PutUint32(h1[12:16], seq1)
		binary.BigEndian.PutUint16(h1[16:18], ttl1)

		// Build header 2
		h2 := make([]byte, 18)
		h2[0] = v2
		h2[1] = f2
		binary.BigEndian.PutUint32(h2[2:6], fl2)
		binary.BigEndian.PutUint16(h2[6:8], val2)
		binary.BigEndian.PutUint32(h2[8:12], ns2)
		binary.BigEndian.PutUint32(h2[12:16], seq2)
		binary.BigEndian.PutUint16(h2[16:18], ttl2)

		// Check for identity
		identical := true
		for i := 0; i < 18; i++ {
			if h1[i] != h2[i] {
				identical = false
				break
			}
		}
		if identical {
			return
		}

		crc1 := encoding.CRC16CCITT(h1)
		crc2 := encoding.CRC16CCITT(h2)

		if crc1 == crc2 {
			t.Logf("LICH-003 MONAD-COLLISION: CRC=0x%04X, h1[0:4]=%X, h2[0:4]=%X",
				crc1, h1[:4], h2[:4])
		}
	})
}

// FuzzCRC16LengthExtension tests whether appending data to a message can
// preserve the CRC value, enabling length-extension attacks.
func FuzzCRC16LengthExtension(f *testing.F) {
	f.Add([]byte{0x01, 0x00, 0x00, 0x01}, []byte{0x00, 0x00})
	f.Add([]byte("test"), []byte{0xFF})
	f.Add(make([]byte, 18), []byte{0xDE, 0xAD})

	f.Fuzz(func(t *testing.T, base, extension []byte) {
		if len(base) == 0 || len(extension) == 0 {
			return
		}

		baseCRC := encoding.CRC16CCITT(base)

		extended := make([]byte, len(base)+len(extension))
		copy(extended, base)
		copy(extended[len(base):], extension)

		extendedCRC := encoding.CRC16CCITT(extended)

		if baseCRC == extendedCRC {
			t.Logf("LICH-003 LENGTH-EXTENSION: CRC=0x%04X preserved after appending %d bytes to %d-byte base",
				baseCRC, len(extension), len(base))
		}
	})
}

// FuzzCRC32MPEG2Collision hunts collisions in the extended CRC-32 checksum.
// CRC-32 has 4 billion values, so collisions are much rarer but still possible.
func FuzzCRC32MPEG2Collision(f *testing.F) {
	f.Add([]byte("input_a"), []byte("input_b"))
	f.Add([]byte{0x00}, []byte{0x01})
	f.Add(make([]byte, 20), append(make([]byte, 19), 0x01))

	f.Fuzz(func(t *testing.T, a, b []byte) {
		if len(a) == 0 || len(b) == 0 {
			return
		}

		crcA := encoding.CRC32MPEG2(a)
		crcB := encoding.CRC32MPEG2(b)

		if crcA == crcB && string(a) != string(b) {
			t.Logf("LICH-003 CRC32-COLLISION: CRC=0x%08X, len(a)=%d, len(b)=%d",
				crcA, len(a), len(b))
		}

		// Verify determinism
		if encoding.CRC32MPEG2(a) != crcA {
			t.Errorf("CRC32MPEG2 not deterministic")
		}
	})
}
