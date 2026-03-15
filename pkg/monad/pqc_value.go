// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package monad

import (
	"encoding/binary"
	"errors"
)

// PQCValueSize is the size of the PQC value in bytes.
const PQCValueSize = 12

// ErrShortBuffer is returned when the input buffer is too short.
var ErrShortBuffer = errors.New("monad: buffer too short for PQC value")

// PQCValue represents the 12-byte PQC authentication value.
// This is carried by reference in the Monad wire format's scratch bytes
// when the S+CUSTOM flags are both set.
type PQCValue struct {
	SigRef  uint32   // Index into Sophia PQC_SIG_MAP BPF map
	KeyRef  uint16   // Index into Sophia PQC_KEY_MAP BPF map
	HashPfx [4]byte  // SHA-256(signature)[0:4] for fast mismatch detection
	SeqNum  uint16   // Per-flow replay protection sequence number
}

// Marshal serializes the PQC value to 12 bytes (network byte order / big-endian).
func (v *PQCValue) Marshal() [PQCValueSize]byte {
	var buf [PQCValueSize]byte
	binary.BigEndian.PutUint32(buf[0:4], v.SigRef)
	binary.BigEndian.PutUint16(buf[4:6], v.KeyRef)
	copy(buf[6:10], v.HashPfx[:])
	binary.BigEndian.PutUint16(buf[10:12], v.SeqNum)
	return buf
}

// UnmarshalPQCValue deserializes 12 bytes into a PQCValue.
func UnmarshalPQCValue(b []byte) (PQCValue, error) {
	if len(b) < PQCValueSize {
		return PQCValue{}, ErrShortBuffer
	}
	var v PQCValue
	v.SigRef = binary.BigEndian.Uint32(b[0:4])
	v.KeyRef = binary.BigEndian.Uint16(b[4:6])
	copy(v.HashPfx[:], b[6:10])
	v.SeqNum = binary.BigEndian.Uint16(b[10:12])
	return v, nil
}

// IsZero returns true if the PQC value is all zeros (no PQC authentication).
func (v *PQCValue) IsZero() bool {
	return v.SigRef == 0 && v.KeyRef == 0 && v.HashPfx == [4]byte{} && v.SeqNum == 0
}
