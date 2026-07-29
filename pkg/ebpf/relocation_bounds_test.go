// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package ebpf

import (
	"math"
	"strings"
	"testing"
)

// TestApplyRelocations_OutOfBoundsOffset covers the bounds check on ELF
// relocation offsets.
//
// reloc.Offset is a uint64 read straight out of the relocation section. The
// original guard was:
//
//	insnIdx := reloc.Offset / 8
//	if int(insnIdx*8+8) > len(insns) { ... }
//
// which fails two ways on a corrupt or hostile object file. insnIdx*8 can wrap
// inside uint64, and converting a value above MaxInt64 to int yields a
// NEGATIVE number — which compares as "in bounds" and then panics on the
// indexing that follows. Either way the caller gets a panic instead of an
// error, and a relocation can be applied to the wrong instruction.
func TestApplyRelocations_OutOfBoundsOffset(t *testing.T) {
	l := &NativeLoader{}
	insns := make([]byte, 64) // 8 instructions

	cases := []struct {
		name   string
		offset uint64
	}{
		{"just past the end", 64},
		{"far past the end", 1 << 20},
		{"wraps uint64 when multiplied", math.MaxUint64/8 + 1},
		{"negative when cast to int", math.MaxUint64},
		{"max int64", math.MaxInt64},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("PANIC on offset %d: %v (bounds check did not hold)", c.offset, r)
				}
			}()

			err := l.applyRelocations(
				insns,
				[]elfRelocation{{Offset: c.offset, Type: 1}},
				map[string]*loadedMap{},
				&parsedELF{},
				map[uint64]int{},
			)
			if err == nil {
				t.Errorf("offset %d should be rejected as out of bounds", c.offset)
				return
			}
			if !strings.Contains(err.Error(), "out of bounds") && !strings.Contains(err.Error(), "aligned") {
				t.Errorf("unexpected error for offset %d: %v", c.offset, err)
			}
		})
	}
}

// TestApplyRelocations_UnalignedOffset asserts a non-multiple-of-8 offset is
// rejected. BPF instructions are 8 bytes; an unaligned relocation would patch
// bytes straddling two instructions.
func TestApplyRelocations_UnalignedOffset(t *testing.T) {
	l := &NativeLoader{}
	insns := make([]byte, 64)

	err := l.applyRelocations(
		insns,
		[]elfRelocation{{Offset: 3, Type: 1}},
		map[string]*loadedMap{},
		&parsedELF{},
		map[uint64]int{},
	)
	if err == nil || !strings.Contains(err.Error(), "aligned") {
		t.Errorf("unaligned offset should be rejected, got %v", err)
	}
}
