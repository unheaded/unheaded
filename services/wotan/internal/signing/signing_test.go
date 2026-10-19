// SPDX-License-Identifier: GPL-3.0-or-later
package signing

import (
	"testing"

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

func TestSignVerifyRoundTrip(t *testing.T) {
	ts := generateTestSigner(t)
	tv, err := NewTopicVerifier(ts.pubKey)
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}

	sig, pub, algo, err := ts.Sign("config.baseline", "heimdall-01", []byte("mjolnir-v1"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	if algo != Algorithm {
		t.Fatalf("expected algorithm %s, got %s", Algorithm, algo)
	}

	err = tv.Verify("config.baseline", "heimdall-01", []byte("mjolnir-v1"), sig, pub, algo)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestRejectTamperedPayload(t *testing.T) {
	ts := generateTestSigner(t)
	tv, _ := NewTopicVerifier(ts.pubKey)

	sig, pub, algo, _ := ts.Sign("config.baseline", "heimdall-01", []byte("original"))
	err := tv.Verify("config.baseline", "heimdall-01", []byte("tampered"), sig, pub, algo)
	if err == nil {
		t.Fatal("expected rejection of tampered payload")
	}
}

func TestRejectWrongAlgorithm(t *testing.T) {
	ts := generateTestSigner(t)
	tv, _ := NewTopicVerifier(ts.pubKey)

	sig, pub, _, _ := ts.Sign("config.baseline", "heimdall-01", []byte("payload"))
	err := tv.Verify("config.baseline", "heimdall-01", []byte("payload"), sig, pub, "hmac-sha256")
	if err != ErrWrongAlgorithm {
		t.Fatalf("expected ErrWrongAlgorithm, got %v", err)
	}
}

func TestRejectWrongSender(t *testing.T) {
	ts := generateTestSigner(t)
	tv, _ := NewTopicVerifier(ts.pubKey)

	sig, pub, algo, _ := ts.Sign("config.baseline", "heimdall-01", []byte("payload"))
	err := tv.Verify("config.baseline", "attacker", []byte("payload"), sig, pub, algo)
	if err == nil {
		t.Fatal("expected rejection of wrong sender")
	}
}

func TestRequiresSignature(t *testing.T) {
	tests := []struct {
		topic string
		want  bool
	}{
		{"config.baseline", true},
		{"config.deltas.node-01", true},
		{"drift.detected.node-01", false},
		{"logs.wotan.info", false},
		{"alerts.critical", false},
	}

	for _, tt := range tests {
		got := RequiresSignature(tt.topic)
		if got != tt.want {
			t.Errorf("RequiresSignature(%q) = %v, want %v", tt.topic, got, tt.want)
		}
	}
}

func TestUnsignedConfigTopicRejected(t *testing.T) {
	tv, _ := NewTopicVerifier()

	// Empty signature on config.* topic
	err := tv.Verify("config.baseline", "sender", []byte("payload"), nil, nil, Algorithm)
	if err == nil {
		t.Fatal("expected rejection of nil signature on config topic")
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

	sig, pub, algo, _ := attacker.Sign("config.baseline", "heimdall-01", []byte("forged"))
	if err := tv.Verify("config.baseline", "heimdall-01", []byte("forged"), sig, pub, algo); err != ErrUntrustedKey {
		t.Fatalf("self-signed message: want ErrUntrustedKey, got %v", err)
	}

	none, _ := NewTopicVerifier()
	if err := none.Verify("config.baseline", "heimdall-01", []byte("forged"), sig, pub, algo); err != ErrNoTrustedKeys {
		t.Fatalf("no trusted keys: want ErrNoTrustedKeys, got %v", err)
	}

	// The trusted key still works.
	sig, pub, algo, _ = trusted.Sign("config.baseline", "heimdall-01", []byte("real"))
	if err := tv.Verify("config.baseline", "heimdall-01", []byte("real"), sig, pub, algo); err != nil {
		t.Fatalf("trusted key: %v", err)
	}
}
