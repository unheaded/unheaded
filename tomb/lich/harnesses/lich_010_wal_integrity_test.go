// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// LICH-010: WAL (Write-Ahead Log) Integrity Fuzzer
//
// Target: pkg/storage/wal — entry encoding, decoding, CRC verification.
// Goal: Find data corruption, CRC bypass, and sequence number confusion
//       in the WAL entry format.
//
// WAL Entry format:
//   [Sequence:8][Length:4][CRC32:4][Data:Length]
//   Header = 16 bytes, CRC32-IEEE over Data only.
//
// Attack vectors:
//   - Truncated entries (< 16 bytes)
//   - CRC mismatches (corrupted data)
//   - Length field overflow (length > remaining buffer)
//   - Sequence number gaps and reordering
//   - Zero-length data entries
//   - Maximum-length data entries
//   - Crafted CRC collisions (two different data with same CRC)

package harnesses

import (
	"encoding/binary"
	"hash/crc32"
	"testing"
)

// WALEntryHeaderSize is the fixed header size for a WAL entry.
const WALEntryHeaderSize = 16

// WALEntry represents a decoded WAL entry.
type WALEntry struct {
	Sequence uint64
	Length   uint32
	CRC     uint32
	Data    []byte
}

// EncodeWALEntry encodes a WAL entry for storage.
func EncodeWALEntry(seq uint64, data []byte) []byte {
	crc := crc32.ChecksumIEEE(data)

	buf := make([]byte, WALEntryHeaderSize+len(data))
	binary.BigEndian.PutUint64(buf[0:8], seq)
	binary.BigEndian.PutUint32(buf[8:12], uint32(len(data)))
	binary.BigEndian.PutUint32(buf[12:16], crc)
	copy(buf[16:], data)

	return buf
}

// DecodeWALEntry decodes a WAL entry from raw bytes.
func DecodeWALEntry(buf []byte) (*WALEntry, error) {
	if len(buf) < WALEntryHeaderSize {
		return nil, ErrWALCorrupted
	}

	entry := &WALEntry{
		Sequence: binary.BigEndian.Uint64(buf[0:8]),
		Length:   binary.BigEndian.Uint32(buf[8:12]),
		CRC:     binary.BigEndian.Uint32(buf[12:16]),
	}

	totalNeeded := WALEntryHeaderSize + int(entry.Length)
	if len(buf) < totalNeeded {
		return nil, ErrWALTruncated
	}

	entry.Data = make([]byte, entry.Length)
	copy(entry.Data, buf[16:totalNeeded])

	// Verify CRC
	computedCRC := crc32.ChecksumIEEE(entry.Data)
	if computedCRC != entry.CRC {
		return nil, ErrWALCRCMismatch
	}

	return entry, nil
}

// WAL error types.
type walError string

func (e walError) Error() string { return string(e) }

const (
	ErrWALCorrupted   walError = "wal: corrupted entry (header too short)"
	ErrWALTruncated   walError = "wal: truncated entry (data shorter than length)"
	ErrWALCRCMismatch walError = "wal: CRC mismatch (data corruption detected)"
)

// WALSequenceTracker tracks sequence numbers for gap detection.
type WALSequenceTracker struct {
	expected uint64
	gaps     int
	reorders int
}

// NewWALSequenceTracker creates a tracker starting at sequence 1.
func NewWALSequenceTracker() *WALSequenceTracker {
	return &WALSequenceTracker{expected: 1}
}

// Track processes a sequence number and detects gaps/reordering.
func (st *WALSequenceTracker) Track(seq uint64) {
	if seq < st.expected {
		st.reorders++
	} else if seq > st.expected {
		st.gaps += int(seq - st.expected)
	}
	if seq >= st.expected {
		st.expected = seq + 1
	}
}

// FuzzWALEntryRoundtrip tests encode/decode roundtrip for WAL entries.
func FuzzWALEntryRoundtrip(f *testing.F) {
	f.Add(uint64(1), []byte("hello"))
	f.Add(uint64(0), []byte{})
	f.Add(uint64(0xFFFFFFFFFFFFFFFF), []byte{0xFF})
	f.Add(uint64(42), make([]byte, 1000))
	f.Add(uint64(1), []byte{0x00})

	// Large data
	large := make([]byte, 4096)
	for i := range large {
		large[i] = byte(i)
	}
	f.Add(uint64(999), large)

	f.Fuzz(func(t *testing.T, seq uint64, data []byte) {
		// Encode
		encoded := EncodeWALEntry(seq, data)

		// Verify encoded size
		expectedSize := WALEntryHeaderSize + len(data)
		if len(encoded) != expectedSize {
			t.Fatalf("encoded size %d, expected %d", len(encoded), expectedSize)
		}

		// Decode
		decoded, err := DecodeWALEntry(encoded)
		if err != nil {
			t.Fatalf("decode error after encode: %v", err)
		}

		// Verify roundtrip
		if decoded.Sequence != seq {
			t.Errorf("sequence roundtrip: %d != %d", decoded.Sequence, seq)
		}
		if int(decoded.Length) != len(data) {
			t.Errorf("length roundtrip: %d != %d", decoded.Length, len(data))
		}
		if string(decoded.Data) != string(data) {
			t.Error("data roundtrip mismatch")
		}

		// Verify CRC
		expectedCRC := crc32.ChecksumIEEE(data)
		if decoded.CRC != expectedCRC {
			t.Errorf("CRC roundtrip: 0x%08X != 0x%08X", decoded.CRC, expectedCRC)
		}
	})
}

// FuzzWALEntryCorruption tests detection of various corruption patterns.
func FuzzWALEntryCorruption(f *testing.F) {
	// Valid entry
	valid := EncodeWALEntry(1, []byte("test data"))
	f.Add(valid)

	// Empty
	f.Add([]byte{})

	// Too short
	f.Add([]byte{0x01, 0x02, 0x03})

	// Header only (no data, but length > 0)
	headerOnly := make([]byte, WALEntryHeaderSize)
	binary.BigEndian.PutUint64(headerOnly[0:8], 1)
	binary.BigEndian.PutUint32(headerOnly[8:12], 100) // claims 100 bytes
	f.Add(headerOnly)

	// Correct header but corrupted data
	corrupted := make([]byte, len(valid))
	copy(corrupted, valid)
	corrupted[len(corrupted)-1] ^= 0xFF // flip last byte
	f.Add(corrupted)

	// All zeros
	f.Add(make([]byte, 32))

	// All 0xFF
	allFF := make([]byte, 32)
	for i := range allFF {
		allFF[i] = 0xFF
	}
	f.Add(allFF)

	// Length overflow (claims MaxUint32 bytes)
	overflow := make([]byte, WALEntryHeaderSize+4)
	binary.BigEndian.PutUint64(overflow[0:8], 1)
	binary.BigEndian.PutUint32(overflow[8:12], 0xFFFFFFFF) // claims 4GB
	f.Add(overflow)

	f.Fuzz(func(t *testing.T, data []byte) {
		entry, err := DecodeWALEntry(data)
		if err != nil {
			// Errors are expected for corrupted input
			return
		}

		// If decode succeeded, the entry must be valid
		if int(entry.Length) != len(entry.Data) {
			t.Errorf("length %d != data len %d", entry.Length, len(entry.Data))
		}

		// CRC must match
		expectedCRC := crc32.ChecksumIEEE(entry.Data)
		if entry.CRC != expectedCRC {
			t.Errorf("CRC mismatch after successful decode: 0x%08X != 0x%08X", entry.CRC, expectedCRC)
		}

		// Re-encode and verify
		reEncoded := EncodeWALEntry(entry.Sequence, entry.Data)
		reDecoded, reErr := DecodeWALEntry(reEncoded)
		if reErr != nil {
			t.Fatalf("re-decode error: %v", reErr)
		}
		if reDecoded.Sequence != entry.Sequence {
			t.Errorf("re-encode sequence mismatch")
		}
	})
}

// FuzzWALSequenceGaps tests sequence tracking and gap detection.
func FuzzWALSequenceGaps(f *testing.F) {
	// Sequential
	f.Add([]byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03,
	})

	// Gap
	f.Add([]byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x05, // gap: missing 2,3,4
	})

	// Reorder
	f.Add([]byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, // out of order
	})

	// Empty
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		st := NewWALSequenceTracker()

		offset := 0
		for offset+8 <= len(data) {
			seq := binary.BigEndian.Uint64(data[offset : offset+8])
			st.Track(seq)
			offset += 8
		}

		// Invariant: gaps and reorders must not be negative
		if st.gaps < 0 {
			t.Errorf("negative gap count: %d", st.gaps)
		}
		if st.reorders < 0 {
			t.Errorf("negative reorder count: %d", st.reorders)
		}
	})
}

// FuzzWALCRC32Collision hunts for CRC-32/IEEE collisions in WAL data.
func FuzzWALCRC32Collision(f *testing.F) {
	f.Add([]byte("data_a"), []byte("data_b"))
	f.Add([]byte{0x00}, []byte{0x01})
	f.Add(make([]byte, 100), append(make([]byte, 99), 0x01))

	f.Fuzz(func(t *testing.T, a, b []byte) {
		if len(a) == 0 || len(b) == 0 {
			return
		}

		crcA := crc32.ChecksumIEEE(a)
		crcB := crc32.ChecksumIEEE(b)

		if crcA == crcB && string(a) != string(b) {
			t.Logf("LICH-010 WAL-CRC32-COLLISION: CRC=0x%08X, len(a)=%d, len(b)=%d",
				crcA, len(a), len(b))
		}
	})
}

// FuzzWALEntryChain tests a sequence of WAL entries for consistency.
func FuzzWALEntryChain(f *testing.F) {
	f.Add(uint64(1), []byte("first"), uint64(2), []byte("second"), uint64(3), []byte("third"))
	f.Add(uint64(0), []byte{}, uint64(0), []byte{}, uint64(0), []byte{})
	f.Add(uint64(100), []byte("a"), uint64(50), []byte("b"), uint64(200), []byte("c"))

	f.Fuzz(func(t *testing.T, seq1 uint64, data1 []byte, seq2 uint64, data2 []byte, seq3 uint64, data3 []byte) {
		// Encode a chain of entries
		entries := []struct {
			seq  uint64
			data []byte
		}{
			{seq1, data1},
			{seq2, data2},
			{seq3, data3},
		}

		var chain []byte
		for _, e := range entries {
			encoded := EncodeWALEntry(e.seq, e.data)
			chain = append(chain, encoded...)
		}

		// Decode the chain
		offset := 0
		decoded := 0
		for offset < len(chain) {
			remaining := chain[offset:]
			entry, err := DecodeWALEntry(remaining)
			if err != nil {
				break
			}

			// Verify against original
			if decoded < len(entries) {
				if entry.Sequence != entries[decoded].seq {
					t.Errorf("chain entry %d: sequence %d != %d", decoded, entry.Sequence, entries[decoded].seq)
				}
				if string(entry.Data) != string(entries[decoded].data) {
					t.Errorf("chain entry %d: data mismatch", decoded)
				}
			}

			offset += WALEntryHeaderSize + int(entry.Length)
			decoded++
		}

		if decoded != len(entries) {
			t.Errorf("decoded %d entries, expected %d", decoded, len(entries))
		}
	})
}
