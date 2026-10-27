// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package signing

import (
	"errors"
	"testing"
	"time"
)

// VerifyStored checks a message Wotan already accepted (read back from its
// WAL, or replicated from a peer's): key and signature, but not the signing
// time against the clock and not the replay cache, since a stored message is
// old by design and is seen again on every catch-up (FND-020).
func TestVerifyStored_AcceptsOldMessagesRepeatedly(t *testing.T) {
	ts := generateTestSigner(t)
	tv, err := NewTopicVerifier(ts.pubKey)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-24 * time.Hour)
	sg, err := ts.SignAt("config.x", "s", []byte("v1"), old)
	if err != nil {
		t.Fatal(err)
	}
	if err := tv.Verify("config.x", "s", []byte("v1"), sg); !errors.Is(err, ErrStale) {
		t.Fatalf("Verify(day-old) = %v, want ErrStale", err)
	}
	for i := 0; i < 2; i++ {
		if err := tv.VerifyStored("config.x", "s", []byte("v1"), sg); err != nil {
			t.Fatalf("VerifyStored #%d = %v, want nil", i+1, err)
		}
	}
	if err := tv.VerifyStored("config.x", "s", []byte("v2"), sg); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("VerifyStored(tampered) = %v, want ErrInvalidSignature", err)
	}
	other := generateTestSigner(t)
	sg2, err := other.Sign("config.x", "s", []byte("v1"))
	if err != nil {
		t.Fatal(err)
	}
	if err := tv.VerifyStored("config.x", "s", []byte("v1"), sg2); !errors.Is(err, ErrUntrustedKey) {
		t.Fatalf("VerifyStored(untrusted key) = %v, want ErrUntrustedKey", err)
	}
	var nilV *TopicVerifier
	if err := nilV.VerifyStored("config.x", "s", []byte("v1"), sg); !errors.Is(err, ErrNoTrustedKeys) {
		t.Fatalf("nil verifier = %v, want ErrNoTrustedKeys (fail closed)", err)
	}
}
