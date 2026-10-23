// SPDX-License-Identifier: GPL-3.0-or-later
package signing

import (
	"testing"
	"time"

	"unheaded/pkg/crypto/pqc"
)

func generateTestSigner(t *testing.T) *TopicSigner {
	t.Helper()
	_, signer, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return NewTopicSigner(signer)
}

func sign(t *testing.T, ts *TopicSigner, topic, sender, payload string) Signed {
	t.Helper()
	sg, err := ts.Sign(topic, sender, []byte(payload))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return sg
}

func TestSignVerifyRoundTrip(t *testing.T) {
	ts := generateTestSigner(t)
	tv, err := NewTopicVerifier(ts.pubKey)
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	sg := sign(t, ts, "config.baseline", "heimdall-01", "mjolnir-v1")
	if sg.Algorithm != Algorithm {
		t.Fatalf("expected algorithm %s, got %s", Algorithm, sg.Algorithm)
	}
	if err := tv.Verify("config.baseline", "heimdall-01", []byte("mjolnir-v1"), sg); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestRejectTamperedPayload(t *testing.T) {
	ts := generateTestSigner(t)
	tv, _ := NewTopicVerifier(ts.pubKey)
	sg := sign(t, ts, "config.baseline", "heimdall-01", "original")
	if err := tv.Verify("config.baseline", "heimdall-01", []byte("tampered"), sg); err != ErrInvalidSignature {
		t.Fatalf("tampered payload: %v", err)
	}
}

func TestRejectWrongAlgorithm(t *testing.T) {
	ts := generateTestSigner(t)
	tv, _ := NewTopicVerifier(ts.pubKey)
	sg := sign(t, ts, "config.baseline", "heimdall-01", "payload")
	sg.Algorithm = "hmac-sha256"
	if err := tv.Verify("config.baseline", "heimdall-01", []byte("payload"), sg); err != ErrWrongAlgorithm {
		t.Fatalf("expected ErrWrongAlgorithm, got %v", err)
	}
}

func TestRejectWrongSender(t *testing.T) {
	ts := generateTestSigner(t)
	tv, _ := NewTopicVerifier(ts.pubKey)
	sg := sign(t, ts, "config.baseline", "heimdall-01", "payload")
	if err := tv.Verify("config.baseline", "attacker", []byte("payload"), sg); err != ErrInvalidSignature {
		t.Fatalf("wrong sender: %v", err)
	}
}

func TestRequiresSignature(t *testing.T) {
	tests := []struct {
		topic string
		want  bool
	}{
		{"config.baseline", true},
		{"config.deltas.node-01", true},
		{"drift.detected.node-01", true}, // ADR-105: drift.* is signed too
		{"logs.wotan.info", false},
		{"alerts.critical", false},
	}
	for _, tt := range tests {
		if got := RequiresSignature(tt.topic); got != tt.want {
			t.Errorf("RequiresSignature(%q) = %v, want %v", tt.topic, got, tt.want)
		}
	}
}

func TestUnsignedConfigTopicRejected(t *testing.T) {
	ts := generateTestSigner(t)
	tv, _ := NewTopicVerifier(ts.pubKey)
	empty := Signed{Algorithm: Algorithm, PublicKey: ts.pubKey, SignedAtUnixMs: time.Now().UnixMilli()}
	if err := tv.Verify("config.baseline", "sender", []byte("payload"), empty); err == nil {
		t.Fatal("expected rejection of an empty signature")
	}
}

// TestRejectSelfSuppliedKey: a publisher that signs with its own fresh key
// and attaches its own public key is refused. Wotan used to verify against
// the key in the request, so this passed and config.* signing proved only
// that the sender owned some key.
func TestRejectSelfSuppliedKey(t *testing.T) {
	trusted := generateTestSigner(t)
	attacker := generateTestSigner(t)
	tv, _ := NewTopicVerifier(trusted.pubKey)

	forged := sign(t, attacker, "config.baseline", "heimdall-01", "forged")
	if err := tv.Verify("config.baseline", "heimdall-01", []byte("forged"), forged); err != ErrUntrustedKey {
		t.Fatalf("self-signed message: want ErrUntrustedKey, got %v", err)
	}
	none, _ := NewTopicVerifier()
	if err := none.Verify("config.baseline", "heimdall-01", []byte("forged"), forged); err != ErrNoTrustedKeys {
		t.Fatalf("no trusted keys: want ErrNoTrustedKeys, got %v", err)
	}
	real := sign(t, trusted, "config.baseline", "heimdall-01", "real")
	if err := tv.Verify("config.baseline", "heimdall-01", []byte("real"), real); err != nil {
		t.Fatalf("trusted key: %v", err)
	}
}

// TestReplayAndStale: the same signed message is accepted once; one signed
// too far from now is refused; and moving its signing time breaks the
// signature (the time is signed).
func TestReplayAndStale(t *testing.T) {
	ts := generateTestSigner(t)
	tv, _ := NewTopicVerifier(ts.pubKey)
	sg := sign(t, ts, "config.baseline", "n1", "v1")
	if err := tv.Verify("config.baseline", "n1", []byte("v1"), sg); err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	// Verify alone records nothing: a refused publish can be retried.
	if err := tv.Verify("config.baseline", "n1", []byte("v1"), sg); err != nil {
		t.Fatalf("verify again before Accept: %v", err)
	}
	if err := tv.Accept(sg); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if err := tv.Verify("config.baseline", "n1", []byte("v1"), sg); err != ErrReplay {
		t.Fatalf("second delivery: want ErrReplay, got %v", err)
	}
	if err := tv.Accept(sg); err != ErrReplay {
		t.Fatalf("second Accept: want ErrReplay, got %v", err)
	}

	old, err := ts.SignAt("config.baseline", "n1", []byte("v0"), time.Now().Add(-MaxSkew-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := tv.Verify("config.baseline", "n1", []byte("v0"), old); err != ErrStale {
		t.Fatalf("stale: want ErrStale, got %v", err)
	}
	old.SignedAtUnixMs = time.Now().UnixMilli() // freshen the time, keep the old signature
	if err := tv.Verify("config.baseline", "n1", []byte("v0"), old); err != ErrInvalidSignature {
		t.Fatalf("re-dated stale message: want ErrInvalidSignature, got %v", err)
	}

	// Once the window has passed, the cache forgets: by then the message is
	// stale anyway.
	tv.now = func() time.Time { return time.Now().Add(MaxSkew + time.Minute) }
	if err := tv.Verify("config.baseline", "n1", []byte("v1"), sg); err != ErrStale {
		t.Fatalf("after the window: want ErrStale, got %v", err)
	}
	if len(tv.seen) != 0 {
		t.Fatalf("expired entries kept: %d", len(tv.seen))
	}
}
