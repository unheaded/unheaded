// SPDX-License-Identifier: GPL-3.0-or-later
// Package signing provides ML-DSA-65 message signing and verification
// for Wotan topic messages. ADR-043 hard condition #2: all config.* topic
// messages MUST be ML-DSA-65 signed before the Mímir's Law spike begins.
package signing

import (
	"crypto/sha256"
	"errors"
	"strings"

	"unheaded/pkg/crypto/pqc"
)

const Algorithm = "ml-dsa-65"

var (
	ErrUnsignedConfigTopic = errors.New("signing: config.* topics require ML-DSA-65 signature")
	ErrInvalidSignature    = errors.New("signing: signature verification failed")
	ErrWrongAlgorithm      = errors.New("signing: only ml-dsa-65 accepted (no algorithm agility)")
)

// TopicSigner signs Wotan topic messages with ML-DSA-65.
type TopicSigner struct {
	signer *pqc.MLDSASigner
	pubKey []byte
}

// NewTopicSigner creates a signer from an ML-DSA-65 key pair.
func NewTopicSigner(signer *pqc.MLDSASigner) *TopicSigner {
	return &TopicSigner{
		signer: signer,
		pubKey: signer.PublicKey(),
	}
}

// Sign produces a signature over the canonical form of a topic message.
// Returns (signature, publicKey, algorithm).
func (ts *TopicSigner) Sign(topic, senderID string, payload []byte) ([]byte, []byte, string, error) {
	canonical := canonicalize(topic, senderID, payload)
	sig, err := ts.signer.Sign(canonical)
	if err != nil {
		return nil, nil, "", err
	}
	return sig, ts.pubKey, Algorithm, nil
}

// TopicVerifier verifies ML-DSA-65 signatures on Wotan topic messages.
type TopicVerifier struct {
	verifier *pqc.MLDSAVerifier
}

// NewTopicVerifier creates a verifier for ML-DSA-65.
func NewTopicVerifier() (*TopicVerifier, error) {
	v, err := pqc.NewMLDSAVerifier(pqc.MLDSA65)
	if err != nil {
		return nil, err
	}
	return &TopicVerifier{verifier: v}, nil
}

// Verify checks a topic message signature.
func (tv *TopicVerifier) Verify(topic, senderID string, payload, signature, publicKey []byte, algorithm string) error {
	if algorithm != Algorithm {
		return ErrWrongAlgorithm
	}
	canonical := canonicalize(topic, senderID, payload)
	ok, err := tv.verifier.Verify(canonical, signature, publicKey)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidSignature
	}
	return nil
}

// RequiresSignature returns true if the given topic requires ML-DSA-65 signing.
// Per ADR-043: all config.* topics must be signed.
func RequiresSignature(topic string) bool {
	return strings.HasPrefix(topic, "config.")
}

// canonicalize produces a deterministic byte representation of a topic message
// for signing: SHA-256(topic || "\x00" || sender_id || "\x00" || payload).
func canonicalize(topic, senderID string, payload []byte) []byte {
	h := sha256.New()
	h.Write([]byte(topic))
	h.Write([]byte{0})
	h.Write([]byte(senderID))
	h.Write([]byte{0})
	h.Write(payload)
	return h.Sum(nil)
}
