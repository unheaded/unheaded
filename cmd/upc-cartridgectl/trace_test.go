// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"strings"
	"testing"

	"unheaded/pkg/cartridge"
)

func TestTraceSlotSHAs(t *testing.T) {
	sha := strings.Repeat("ab", 32)
	slots := map[int]SlotState{0: {SHA256: sha}, 2: {SHA256: "not-hex"}, 9: {SHA256: sha}}
	shas := slotSHAs(slots)
	trace := cartridge.EncodeTrace(shas, nil)
	got, err := traceSlotSHA(trace, 0)
	if err != nil || got != sha {
		t.Fatalf("slot 0: %q %v", got, err)
	}
	for _, s := range []int{1, 2, 3} {
		if got, err := traceSlotSHA(trace, s); err != nil || got != "" {
			t.Fatalf("slot %d: %q %v (unknown slots carry no sha)", s, got, err)
		}
	}
	if _, err := traceSlotSHA(trace, 4); err == nil {
		t.Fatal("slot 4 accepted")
	}
	if _, err := traceSlotSHA([]byte("UCTR"), 0); err == nil {
		t.Fatal("short trace accepted")
	}
	if _, err := traceSlotSHA(append([]byte("XXXX"), trace[4:]...), 0); err == nil {
		t.Fatal("bad magic accepted")
	}
}
