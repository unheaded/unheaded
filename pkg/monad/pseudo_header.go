// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package monad

// PseudoHeaderSize is the size of the pseudo-header for PQC signing.
const PseudoHeaderSize = 52

// PseudoHeader is the data that gets signed for PQC authentication.
//
// Layout (52 bytes):
//
//	Byte  0-15:  Source IPv6 address (16 bytes)
//	Byte 16-31:  Destination IPv6 address (16 bytes)
//	Byte 32-39:  Monad bytes [0:8] — version through flags
//	Byte 40-51:  PQC value [0:12]
type PseudoHeader struct {
	SrcIP    [16]byte // Source IPv6 address
	DstIP    [16]byte // Destination IPv6 address
	Monad    [8]byte  // Monad bytes 0-7 (version, src/dst svc, hop, qos, action, circuit, flags)
	PQCValue [12]byte // The PQC value being authenticated
}

// Marshal serializes to 52 bytes for signing.
func (ph *PseudoHeader) Marshal() [PseudoHeaderSize]byte {
	var buf [PseudoHeaderSize]byte
	copy(buf[0:16], ph.SrcIP[:])
	copy(buf[16:32], ph.DstIP[:])
	copy(buf[32:40], ph.Monad[:])
	copy(buf[40:52], ph.PQCValue[:])
	return buf
}

// BuildPseudoHeader constructs a pseudo-header from components.
func BuildPseudoHeader(srcIP, dstIP [16]byte, monadPrefix [8]byte, pqcValue [12]byte) *PseudoHeader {
	return &PseudoHeader{
		SrcIP:    srcIP,
		DstIP:    dstIP,
		Monad:    monadPrefix,
		PQCValue: pqcValue,
	}
}
