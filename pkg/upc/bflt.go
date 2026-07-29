// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package upc

import (
	"encoding/binary"
	"fmt"
)

// UPCFlat -- simplified bFLT binary format for the UPC.
//
// Real bFLT (uClinux) has relocations, compression flags, etc. UPCFlat strips
// that down to the minimum needed for a nommu flat binary: a header, a text
// segment (instructions), and a data segment, plus BSS size for zero-fill.
//
// On-disk layout:
//
//	Bytes 0-31:   UPCFlatHeader (8 x uint32, little-endian)
//	Bytes 32...:  Text words (TextSize x 4 bytes)
//	              Data words (DataSize x 4 bytes)

// UPCFlatHeader is the 32-byte header at the start of a .upcf binary.
type UPCFlatHeader struct {
	Magic     [4]byte // "UPCF"
	Version   uint32  // 1
	Entry     uint32  // Entry point (word offset in text)
	TextSize  uint32  // Text segment size in words
	DataStart uint32  // Data segment start (word offset from binary start, after header)
	DataSize  uint32  // Data segment size in words
	BSSSize   uint32  // BSS size in words (zero-filled after data)
	StackSize uint32  // Requested stack size in bytes
}

const (
	// UPCFlatMagic is the 4-byte magic: "UPCF".
	UPCFlatMagic = "UPCF"

	// UPCFlatVersion is the current format version.
	UPCFlatVersion = 1

	// UPCFlatHeaderSize is the header size in bytes (8 fields x 4 bytes).
	UPCFlatHeaderSize = 32
)

// CreateUPCFlat builds a UPC flat binary from text (instructions) and data segments.
// The returned byte slice contains the header followed by text and data words,
// all in little-endian format.
func CreateUPCFlat(text []uint32, data []uint32, bssWords uint32, stackSize uint32) []byte {
	hdr := UPCFlatHeader{
		Version:  UPCFlatVersion,
		Entry:    0,                 // default entry at first text word
		TextSize: uint32(len(text)), // #nosec G115 -- bounded by the length of an already-allocated slice
		// #nosec G115 -- bounded by the length of an already-allocated slice
		DataStart: uint32(len(text)), // data follows text, in words from payload start
		DataSize:  uint32(len(data)), // #nosec G115 -- bounded by the length of an already-allocated slice
		BSSSize:   bssWords,
		StackSize: stackSize,
	}
	copy(hdr.Magic[:], UPCFlatMagic)

	totalBytes := UPCFlatHeaderSize + (len(text)+len(data))*4
	out := make([]byte, totalBytes)

	// Write header
	copy(out[0:4], hdr.Magic[:])
	binary.LittleEndian.PutUint32(out[4:8], hdr.Version)
	binary.LittleEndian.PutUint32(out[8:12], hdr.Entry)
	binary.LittleEndian.PutUint32(out[12:16], hdr.TextSize)
	binary.LittleEndian.PutUint32(out[16:20], hdr.DataStart)
	binary.LittleEndian.PutUint32(out[20:24], hdr.DataSize)
	binary.LittleEndian.PutUint32(out[24:28], hdr.BSSSize)
	binary.LittleEndian.PutUint32(out[28:32], hdr.StackSize)

	// Write text segment
	off := UPCFlatHeaderSize
	for _, w := range text {
		binary.LittleEndian.PutUint32(out[off:off+4], w)
		off += 4
	}

	// Write data segment
	for _, w := range data {
		binary.LittleEndian.PutUint32(out[off:off+4], w)
		off += 4
	}

	return out
}

// ParseUPCFlat parses a UPC flat binary, returning the header plus the text
// and data word slices extracted from the payload.
func ParseUPCFlat(bin []byte) (*UPCFlatHeader, []uint32, []uint32, error) {
	if len(bin) < UPCFlatHeaderSize {
		return nil, nil, nil, fmt.Errorf("upcflat: binary too short (%d bytes, need >= %d)", len(bin), UPCFlatHeaderSize)
	}

	// Check magic
	if string(bin[0:4]) != UPCFlatMagic {
		return nil, nil, nil, fmt.Errorf("upcflat: invalid magic %q (want %q)", string(bin[0:4]), UPCFlatMagic)
	}

	hdr := &UPCFlatHeader{}
	copy(hdr.Magic[:], bin[0:4])
	hdr.Version = binary.LittleEndian.Uint32(bin[4:8])
	hdr.Entry = binary.LittleEndian.Uint32(bin[8:12])
	hdr.TextSize = binary.LittleEndian.Uint32(bin[12:16])
	hdr.DataStart = binary.LittleEndian.Uint32(bin[16:20])
	hdr.DataSize = binary.LittleEndian.Uint32(bin[20:24])
	hdr.BSSSize = binary.LittleEndian.Uint32(bin[24:28])
	hdr.StackSize = binary.LittleEndian.Uint32(bin[28:32])

	if hdr.Version != UPCFlatVersion {
		return nil, nil, nil, fmt.Errorf("upcflat: unsupported version %d (want %d)", hdr.Version, UPCFlatVersion)
	}

	// Validate payload sizes
	payloadBytes := len(bin) - UPCFlatHeaderSize
	neededBytes := int(hdr.TextSize+hdr.DataSize) * 4
	if payloadBytes < neededBytes {
		return nil, nil, nil, fmt.Errorf("upcflat: payload too short (%d bytes, need %d for text+data)", payloadBytes, neededBytes)
	}

	// Extract text words
	text := make([]uint32, hdr.TextSize)
	off := UPCFlatHeaderSize
	for i := range text {
		text[i] = binary.LittleEndian.Uint32(bin[off : off+4])
		off += 4
	}

	// Extract data words
	data := make([]uint32, hdr.DataSize)
	for i := range data {
		data[i] = binary.LittleEndian.Uint32(bin[off : off+4])
		off += 4
	}

	return hdr, text, data, nil
}

// LoadUPCFlat loads a UPC flat binary into CPU memory.
//
// Text is copied into rom starting at word 0. Data is copied into ram
// starting at dataAddr (word address). BSS words are zeroed immediately
// after the data segment in ram.
//
// Returns the parsed header so the caller can set PC = header.Entry and
// configure the stack from header.StackSize.
func LoadUPCFlat(bin []byte, rom []uint32, ram []uint32, dataAddr uint32) (*UPCFlatHeader, error) {
	hdr, text, data, err := ParseUPCFlat(bin)
	if err != nil {
		return nil, err
	}

	// Load text into ROM
	if int(hdr.TextSize) > len(rom) {
		return nil, fmt.Errorf("upcflat: text segment (%d words) exceeds ROM capacity (%d words)", hdr.TextSize, len(rom))
	}
	copy(rom, text)

	// Load data into RAM at dataAddr
	dataEnd := dataAddr + hdr.DataSize
	if int(dataEnd) > len(ram) {
		return nil, fmt.Errorf("upcflat: data segment end (%d) exceeds RAM capacity (%d words)", dataEnd, len(ram))
	}
	for i, w := range data {
		ram[dataAddr+uint32(i)] = w
	}

	// Zero BSS immediately after data
	bssStart := dataEnd
	bssEnd := bssStart + hdr.BSSSize
	if int(bssEnd) > len(ram) {
		return nil, fmt.Errorf("upcflat: BSS end (%d) exceeds RAM capacity (%d words)", bssEnd, len(ram))
	}
	for i := bssStart; i < bssEnd; i++ {
		ram[i] = 0
	}

	return hdr, nil
}
