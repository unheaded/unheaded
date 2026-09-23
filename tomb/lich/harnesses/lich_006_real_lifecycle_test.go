// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// LICH-006 (real target): pkg/protocol/lifecycle
//
// lich_006_state_machine_test.go declares its target as "Monad state machine
// transitions (services/monad, pkg/protocol/lifecycle)" and imports neither.
// It has a FuzzGoawayMonotonicity that exercises a state machine defined
// inside the test file, so it cannot find a defect in the shipping code.
// Third instance of the audit in ADR-062; see ADR-093 for why this keeps
// happening.
//
// pkg/protocol/lifecycle IS reachable from here — it is under pkg/, not
// inside an internal/ tree — so unlike LICH-010 this harness can stay in
// tomb/.
//
// Target: the GOAWAY / CANCEL_FLOW wire frames and the monotonicity rule
// they enforce. GOAWAY carries "I will serve no flow above N"; a decoder
// that accepts a frame it should not, or a tracker that lets LastFlowID go
// backwards, means a peer can reopen flows the server has already promised
// to drain. Both frames are fixed 6-byte layouts read off the wire.
//
// Invariants:
//   - Decode never panics on arbitrary bytes and never returns (nil, nil)
//   - Decode rejects anything shorter than the fixed 6-byte frame
//   - Encode -> Decode round-trips both frame types exactly
//   - Decode ignores trailing bytes rather than misreading the prefix
//   - The tracker never allows LastFlowID to decrease once initialised
//   - ValidateMonotonicity agrees with a sequence replayed through a tracker
package harnesses

import (
	"testing"

	"unheaded/pkg/protocol/lifecycle"
)

// FuzzRealGoawayDecode feeds arbitrary bytes to the GOAWAY frame decoder.
func FuzzRealGoawayDecode(f *testing.F) {
	f.Add([]byte{0, 0, 0, 0, 0, 0})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	f.Add([]byte{1, 2, 3, 4, 5})          // one short
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8}) // trailing bytes
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		frame, err := lifecycle.DecodeGoawayFrame(data)

		if err == nil && frame == nil {
			t.Fatal("DecodeGoawayFrame returned (nil, nil)")
		}
		if err != nil {
			if frame != nil {
				t.Fatalf("returned both a frame and an error: %v", err)
			}
			return
		}

		// A 6-byte fixed layout: anything shorter must have been rejected.
		if len(data) < 6 {
			t.Fatalf("accepted a %d-byte buffer for a 6-byte frame", len(data))
		}

		// Re-encoding must reproduce the first 6 bytes exactly. Trailing
		// bytes are ignored, not folded into the fields.
		reencoded, err := lifecycle.EncodeGoawayFrame(frame)
		if err != nil {
			t.Fatalf("re-encode of a decoded frame failed: %v", err)
		}
		if string(reencoded) != string(data[:6]) {
			t.Fatalf("re-encode differs from the decoded prefix\n in: %x\nout: %x", data[:6], reencoded)
		}
	})
}

// FuzzRealCancelFlowDecode does the same for CANCEL_FLOW.
func FuzzRealCancelFlowDecode(f *testing.F) {
	f.Add([]byte{0, 0, 0, 0, 0, 0})
	f.Add([]byte{0xde, 0xad, 0xbe, 0xef, 0, 1})
	f.Add([]byte{1, 2, 3})
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7})

	f.Fuzz(func(t *testing.T, data []byte) {
		frame, err := lifecycle.DecodeCancelFlowFrame(data)

		if err == nil && frame == nil {
			t.Fatal("DecodeCancelFlowFrame returned (nil, nil)")
		}
		if err != nil {
			if frame != nil {
				t.Fatalf("returned both a frame and an error: %v", err)
			}
			return
		}
		if len(data) < 6 {
			t.Fatalf("accepted a %d-byte buffer for a 6-byte frame", len(data))
		}

		reencoded, err := lifecycle.EncodeCancelFlowFrame(frame)
		if err != nil {
			t.Fatalf("re-encode of a decoded frame failed: %v", err)
		}
		if string(reencoded) != string(data[:6]) {
			t.Fatalf("re-encode differs from the decoded prefix\n in: %x\nout: %x", data[:6], reencoded)
		}
	})
}

// FuzzRealGoawayMonotonicity drives the tracker with an arbitrary sequence of
// flow IDs. GOAWAY promises the peer that no flow above LastFlowID will be
// served; letting that number decrease would take the promise back.
func FuzzRealGoawayMonotonicity(f *testing.F) {
	f.Add([]uint8{1, 2, 3})
	f.Add([]uint8{3, 2, 1})
	f.Add([]uint8{5, 5, 5})
	f.Add([]uint8{0})
	f.Add([]uint8{})

	f.Fuzz(func(t *testing.T, raw []uint8) {
		if len(raw) > 512 {
			raw = raw[:512]
		}

		tracker := lifecycle.NewGoawayTracker()
		ids := make([]uint32, 0, len(raw))

		var highest uint32
		var started bool
		for _, b := range raw {
			id := uint32(b)
			err := tracker.ValidateAndUpdate(&lifecycle.GoawayFrame{LastFlowID: id})

			switch {
			case !started:
				// First frame initialises the tracker; anything is allowed.
				if err != nil {
					t.Fatalf("first GOAWAY (%d) rejected: %v", id, err)
				}
				started, highest = true, id
				ids = append(ids, id)
			case id < highest:
				if err == nil {
					t.Fatalf("tracker accepted a decrease: %d after %d", id, highest)
				}
				// Rejected: the tracker must not have moved.
				if got := tracker.LastFlowID(); got != highest {
					t.Fatalf("rejected frame still moved the tracker: %d -> %d", highest, got)
				}
			default:
				if err != nil {
					t.Fatalf("tracker rejected a non-decreasing id %d after %d: %v", id, highest, err)
				}
				highest = id
				ids = append(ids, id)
			}

			if got := tracker.LastFlowID(); got != highest {
				t.Fatalf("LastFlowID() = %d, want %d", got, highest)
			}
		}

		// The accepted sequence is monotonic by construction, so the
		// standalone validator must agree. Two implementations of the same
		// rule disagreeing is the defect this catches.
		if err := lifecycle.ValidateMonotonicity(ids); err != nil {
			t.Fatalf("ValidateMonotonicity rejected a sequence the tracker accepted: %v (%v)", err, ids)
		}
	})
}

// FuzzRealValidateMonotonicity checks the standalone validator directly.
func FuzzRealValidateMonotonicity(f *testing.F) {
	f.Add([]uint8{1, 2, 3})
	f.Add([]uint8{2, 1})
	f.Add([]uint8{})

	f.Fuzz(func(t *testing.T, raw []uint8) {
		if len(raw) > 512 {
			raw = raw[:512]
		}
		ids := make([]uint32, len(raw))
		for i, b := range raw {
			ids[i] = uint32(b)
		}

		err := lifecycle.ValidateMonotonicity(ids)

		// Independent check: is the sequence actually non-decreasing?
		descending := false
		for i := 1; i < len(ids); i++ {
			if ids[i] < ids[i-1] {
				descending = true
				break
			}
		}

		if descending && err == nil {
			t.Fatalf("accepted a descending sequence: %v", ids)
		}
		if !descending && err != nil {
			t.Fatalf("rejected a non-decreasing sequence %v: %v", ids, err)
		}
	})
}
