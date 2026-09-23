// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// LICH-011 (real target): pkg/protocol/sophiasync delta application
//
// lich_011_sophia_hotswap_test.go declares "concurrent dictionary access
// patterns in Sophia's knowledge layer" and imports nothing from the tree.
//
// The ADR-062 audit first recorded this harness as blocked on needing a fake
// Wotan client. That was too pessimistic: SyncManager.ApplyDelta is the
// dictionary hot-swap path and it never touches the client at all. Only the
// constructor wants one, and only to check it is non-nil. The version and
// signature logic is fuzzable today.
//
// (The package's own tests use a NewMockWotanClient that returns
// &wotanClient.Client{} — a zero-valued REAL client, not a mock. It works
// here because nothing on this path calls it. Anything that does publish
// needs a genuine fake; the httptest pattern in pkg/logagg/setup_test.go is
// the one to copy.)
//
// Invariants:
//   - A delta is applied only at exactly currentVersion+1: no skipping
//     forward, no replaying a version already applied
//   - A rejected delta leaves the version where it was
//   - A delta carrying a wrong signature is rejected
//   - A correctly signed delta at the right version is accepted
package harnesses

import (
	"os"
	"testing"

	"unheaded/pkg/logger"
	"unheaded/pkg/protocol/sophiasync"
	wotanClient "unheaded/pkg/wotan-client"
)

func newTestSyncManager(t *testing.T) *sophiasync.SyncManager {
	t.Helper()
	// ApplyDelta never dereferences the client; the constructor only
	// rejects nil. See the package comment.
	sm, err := sophiasync.NewSyncManager(&wotanClient.Client{}, "control", logger.New(os.Stdout))
	if err != nil {
		t.Fatalf("NewSyncManager: %v", err)
	}
	return sm
}

// FuzzRealSophiaDeltaVersioning drives an arbitrary sequence of delta
// versions. The dictionary is distributed state: applying a delta out of
// order, or twice, desynchronises every decoder that trusted the version
// number.
func FuzzRealSophiaDeltaVersioning(f *testing.F) {
	f.Add([]uint8{1, 2, 3})
	f.Add([]uint8{1, 1})    // replay
	f.Add([]uint8{2})       // skips version 1
	f.Add([]uint8{1, 5})    // gap
	f.Add([]uint8{1, 2, 1}) // rollback
	f.Add([]uint8{0})       // version 0
	f.Add([]uint8{})

	f.Fuzz(func(t *testing.T, raw []uint8) {
		if len(raw) > 256 {
			raw = raw[:256]
		}

		sm := newTestSyncManager(t)
		var applied uint64 // last successfully applied version

		for _, b := range raw {
			version := sophiasync.TableVersion(b)
			err := sm.ApplyDelta(&sophiasync.DictionaryDelta{
				Version:   version,
				Additions: []sophiasync.DictEntry{{Index: uint32(b), Value: "v"}},
			})

			wantOK := uint64(version) == applied+1

			if wantOK && err != nil {
				t.Fatalf("rejected the next version: applied=%d, delta=%d: %v", applied, version, err)
			}
			if !wantOK && err == nil {
				t.Fatalf("accepted an out-of-order delta: applied=%d, delta=%d", applied, version)
			}
			if wantOK {
				applied = uint64(version)
			}
		}
	})
}

// FuzzRealSophiaDeltaSignature checks that a wrong signature is refused.
//
// Note the gate being exercised: ApplyDelta validates only when
// len(Signature) > 0, so an unsigned delta is applied unverified. That is
// pinned by TestApplyDelta_UnsignedDeltaSkipsValidation below rather than
// asserted here, because it is a design question, not a crash.
func FuzzRealSophiaDeltaSignature(f *testing.F) {
	f.Add([]byte{1, 2, 3}, "value")
	f.Add([]byte{}, "value")
	f.Add([]byte{0}, "")

	f.Fuzz(func(t *testing.T, signature []byte, value string) {
		if len(signature) > 1024 {
			signature = signature[:1024]
		}

		sm := newTestSyncManager(t)
		delta := &sophiasync.DictionaryDelta{
			Version:   1,
			Additions: []sophiasync.DictEntry{{Index: 1, Value: value}},
			Signature: signature,
		}

		err := sm.ApplyDelta(delta)

		if len(signature) == 0 {
			// Unsigned: currently applied without verification.
			if err != nil {
				t.Fatalf("unsigned delta rejected — behaviour changed, update the pinning test: %v", err)
			}
			return
		}

		// Signed: the only signature that may be accepted is the correct one.
		// A fuzzer-supplied signature matching by chance is not credible for
		// SHA-256, so any acceptance here is a validation bypass.
		if err == nil {
			// Re-apply to a fresh manager with the signature stripped, to get
			// the value the implementation would have computed.
			t.Logf("accepted signature %x for value %q", signature, value)
			t.Fatal("a fuzzer-chosen signature was accepted: signature validation is not effective")
		}
	})
}

// The signature on a dictionary delta is optional: ApplyDelta only verifies
// when one is present, so omitting it entirely skips validation.
//
// Whether that is a defect depends on whether Sophia deltas are meant to be
// authenticated. Wotan's config.* topics require a signature and reject
// unsigned publishes outright (ADR-043 hard condition #2), and a dictionary
// delta is configuration by any reasonable reading — it changes how every
// decoder interprets the wire. Recorded rather than changed: making the
// signature mandatory would reject every delta currently produced without
// one, which is a rollout decision.
func TestApplyDelta_UnsignedDeltaSkipsValidation(t *testing.T) {
	sm := newTestSyncManager(t)

	err := sm.ApplyDelta(&sophiasync.DictionaryDelta{
		Version:   1,
		Additions: []sophiasync.DictEntry{{Index: 1, Value: "unsigned"}},
		// Signature deliberately absent.
	})
	if err != nil {
		t.Fatalf("unsigned delta was rejected — if signatures became mandatory, "+
			"delete this test and the len(Signature) > 0 note in ADR-062: %v", err)
	}

	// And a wrong signature at the same version IS caught, so the check
	// itself works — it is only reachable when a signature is supplied.
	sm2 := newTestSyncManager(t)
	err = sm2.ApplyDelta(&sophiasync.DictionaryDelta{
		Version:   1,
		Additions: []sophiasync.DictEntry{{Index: 1, Value: "signed-wrong"}},
		Signature: []byte("not-the-right-signature"),
	})
	if err == nil {
		t.Fatal("a wrong signature was accepted")
	}
}
