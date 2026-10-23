// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"encoding/hex"
	"errors"
	"fmt"

	"unheaded/pkg/cartridge"
)

// slotSHAs turns slots.json into the per-slot .cart sha256 a trace file
// carries (zero for an empty slot or an unreadable sha).
func slotSHAs(slots map[int]SlotState) [cartridge.Slots][32]byte {
	var out [cartridge.Slots][32]byte
	for s, st := range slots {
		if s < 0 || s >= cartridge.Slots {
			continue
		}
		b, err := hex.DecodeString(st.SHA256)
		if err == nil && len(b) == 32 {
			copy(out[s][:], b)
		}
	}
	return out
}

// traceSlotSHA reads the recorded .cart sha256 of slot from a UCTR trace
// header ("" when the trace does not know it).
func traceSlotSHA(trace []byte, slot int) (string, error) {
	if slot < 0 || slot >= cartridge.Slots {
		return "", fmt.Errorf("slot %d outside 0..%d", slot, cartridge.Slots-1)
	}
	if len(trace) < 8+32*cartridge.Slots || string(trace[:4]) != "UCTR" {
		return "", errors.New("not a UCTR trace")
	}
	sha := trace[8+32*slot : 8+32*(slot+1)]
	for _, b := range sha {
		if b != 0 {
			return hex.EncodeToString(sha), nil
		}
	}
	return "", nil
}
