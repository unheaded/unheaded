// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package ebpf

import "testing"

// S-TRUE-WIRE Step 386: the legacy decoder reads ring-buffer records from
// pre-sprint objects. It must not panic, must refuse a zero timestamp like
// the spec decoder, and must agree with it on every field but the type.
func FuzzDecodeAnamnesisEventLegacy(f *testing.F) {
	f.Add(make([]byte, AnamnesisEventSize))
	f.Add(spec32(1000, 0x01))
	f.Add(spec32(1000, 0x08))
	f.Add([]byte{1})
	f.Fuzz(func(t *testing.T, b []byte) {
		ev, err := DecodeAnamnesisEventLegacy(b)
		if err != nil {
			return
		}
		if ev.TimestampNs == 0 {
			t.Fatal("legacy decoder accepted a zero timestamp")
		}
		if !ev.EventType.Valid() {
			t.Fatalf("legacy decoder produced unregistered type %d", ev.EventType)
		}
		spec, err := DecodeAnamnesisEvent(b)
		if err != nil {
			return // the byte means something else (or nothing) in the spec numbering
		}
		if spec.TimestampNs != ev.TimestampNs || spec.HopID != ev.HopID || spec.FlowLabelLo != ev.FlowLabelLo || spec.Monad != ev.Monad {
			t.Fatalf("decoders disagree beyond the type: %+v vs %+v", spec, ev)
		}
	})
}
