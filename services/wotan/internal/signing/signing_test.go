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
	tv, err := NewTopicVerifier()
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
	tv, _ := NewTopicVerifier()

	sig, pub, algo, _ := ts.Sign("config.baseline", "heimdall-01", []byte("original"))
	err := tv.Verify("config.baseline", "heimdall-01", []byte("tampered"), sig, pub, algo)
	if err == nil {
		t.Fatal("expected rejection of tampered payload")
	}
}

func TestRejectWrongAlgorithm(t *testing.T) {
	ts := generateTestSigner(t)
	tv, _ := NewTopicVerifier()

	sig, pub, _, _ := ts.Sign("config.baseline", "heimdall-01", []byte("payload"))
	err := tv.Verify("config.baseline", "heimdall-01", []byte("payload"), sig, pub, "hmac-sha256")
	if err != ErrWrongAlgorithm {
		t.Fatalf("expected ErrWrongAlgorithm, got %v", err)
	}
}

func TestRejectWrongSender(t *testing.T) {
	ts := generateTestSigner(t)
	tv, _ := NewTopicVerifier()

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
