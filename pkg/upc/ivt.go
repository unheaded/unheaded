// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package upc

// IVTEntry represents one interrupt vector table entry.
type IVTEntry struct {
	Vector  uint8
	Handler uint32
}

// DefaultIVT returns the default interrupt vector table.
// All 256 vectors point to a default HLT handler at DefaultHandlerAddr
// until the kernel installs its own handlers.
func DefaultIVT() []IVTEntry {
	entries := make([]IVTEntry, IVTEntries)
	for i := range entries {
		entries[i] = IVTEntry{
			Vector:  uint8(i),
			Handler: DefaultHandlerAddr,
		}
	}
	return entries
}

// IVTToWords converts the IVT to a word slice for writing to RAM.
// IVT lives at RAM[0x0000-0x03FF] (256 entries x 4 bytes each).
// The returned slice is indexed by vector number.
func IVTToWords(entries []IVTEntry) []uint32 {
	words := make([]uint32, IVTEntries)
	for _, e := range entries {
		words[e.Vector] = e.Handler
	}
	return words
}
